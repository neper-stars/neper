package handlers

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/go-openapi/runtime/middleware"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/lib/registration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewRegisterHandler creates a new handler for user registration
func NewRegisterHandler(log *zerolog.Logger, db *sqlx.DB, opts *registration.Options, limiter *registration.RateLimiter, notifyService *notify.Service) *RegisterHandler {
	return &RegisterHandler{db: db, log: log, opts: opts, limiter: limiter, notifyService: notifyService}
}

// RegisterHandler handles POST /auth/register
type RegisterHandler struct {
	db            *sqlx.DB
	log           *zerolog.Logger
	opts          *registration.Options
	limiter       *registration.RateLimiter
	notifyService *notify.Service
}

func (h *RegisterHandler) handle(
	ctx context.Context, params operations.RegisterParams,
) (*models.UserProfile, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// Validate input
	if params.Registration == nil {
		return nil, errs.NewErrInvalidSomething("registration data is required")
	}
	if strings.TrimSpace(params.Registration.Nickname) == "" {
		return nil, errs.NewErrInvalidSomething("nickname is required")
	}
	if strings.TrimSpace(params.Registration.Email) == "" {
		return nil, errs.NewErrInvalidSomething("email is required")
	}

	// Create the pending user profile
	// - pending=true: needs approval
	// - is_active=false: cannot authenticate
	// - no API key: will be generated on approval
	userProfileDB := models.UserProfileDB{
		UserProfile: models.UserProfile{
			ID:                  uuid.New().String(),
			Nickname:            strings.TrimSpace(params.Registration.Nickname),
			Email:               strings.TrimSpace(params.Registration.Email),
			IsActive:            false,
			IsManager:           false,
			Pending:             true,
			RegistrationMessage: params.Registration.Message,
		},
	}

	if _, err := sqlH.Insert(&userProfileDB); err != nil {
		// Check for unique constraint violation
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return nil, errs.NewErrAlreadyExists("nickname or email already exists")
		}
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	log.Info().
		Str("user_id", userProfileDB.ID).
		Str("nickname", userProfileDB.Nickname).
		Str("email", userProfileDB.Email).
		Msg("new pending registration created")

	// Notify global managers about the new pending registration
	if h.notifyService != nil {
		if err := h.notifyService.PublishPendingRegistrationCreate(userProfileDB.ID); err != nil {
			log.Err(err).Msg("failed to publish pending registration notification")
		}
	}

	return &userProfileDB.UserProfile, nil
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies)
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// Take the first IP in the chain
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// Handle handles the request
func (h *RegisterHandler) Handle(
	params operations.RegisterParams,
) middleware.Responder {
	log := zerolog.Ctx(params.HTTPRequest.Context())

	// Check if registration is enabled
	if h.opts != nil && !h.opts.Enabled {
		msg := "registration is currently disabled"
		log.Info().Msg("registration attempt rejected: disabled")
		return operations.NewRegisterServiceUnavailable().WithPayload(&models.Error{
			Code:    http.StatusServiceUnavailable,
			Message: &msg,
		})
	}

	// Check rate limit
	if h.limiter != nil {
		clientIP := getClientIP(params.HTTPRequest)
		if !h.limiter.Allow(clientIP) {
			msg := "too many registration attempts, please try again later"
			log.Warn().Str("client_ip", clientIP).Msg("registration rate limited")
			return operations.NewRegisterTooManyRequests().WithPayload(&models.Error{
				Code:    http.StatusTooManyRequests,
				Message: &msg,
			})
		}
	}

	userProfile, err := h.handle(params.HTTPRequest.Context(), params)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrConflict):
			msg := err.Error()
			return operations.NewRegisterConflict().WithPayload(&models.Error{
				Code:    http.StatusConflict,
				Message: &msg,
			})
		case errors.Is(err, errs.ErrInvalid):
			return BadRequest(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewRegisterCreated().WithPayload(userProfile)
}
