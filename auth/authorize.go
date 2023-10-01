package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	sq "github.com/Masterminds/squirrel"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

const (
	XAuthNamespace = "x-auth-namespace"
	XAuthRelation  = "x-auth-relation"

	RelationCreate = "create"
	RelationRead   = "read"
	RelationUpdate = "update"
	RelationDelete = "delete"

	// TODO: grab the namespace from the API route instead of hard coded
	// TODO: use code generation to set the consts here

	NamespaceUsers       = "users"
	NamespaceSessions    = "sessions"
	NamespaceRaces       = "races"
	NamespaceToken       = "tokens"
	NamespaceInvitations = "invitations"
)

var (
	ErrAuthZFailed   = fmt.Errorf("authorization failed")
	ErrAuthZInternal = fmt.Errorf("authorization internal failure")
)

// Authorizer will provide the api authorizer through the Authorize
// function. It will also provide basic structural authorizations setup
// for all authorizations that are not derived from dynamic values.
// for example all the root namespace will be populated this way
// versus a specific /api/v1/sessions/sessionID1 object which will be
// handled in the model part
type Authorizer struct {
	logger zerolog.Logger
	db     *sqlx.DB
}

// NewAuthorizer returns an Authorizer ready to be used
func NewAuthorizer(log zerolog.Logger, db *sqlx.DB) *Authorizer {
	log.Debug().Msg("starting up the Authorizer")
	return &Authorizer{
		logger: log,
		db:     db,
	}
}

// GetTestAuthorizer returns a test Authorizer
func GetTestAuthorizer(log zerolog.Logger, db *sqlx.DB) *Authorizer {
	return NewAuthorizer(log, db)
}

// Authorize is the closure that prepares the authorizer for our restapi
// every authenticated request will also be evaluated through the returned
// function (which implements the runtime.Authorizer interface)
// Our authorizer will then inspect the requested resource vs the principal
// permissions and decide if the principal permissions allow the request
// to continue or not
func (d *Authorizer) Authorize(mc *middleware.Context) runtime.Authorizer {
	return runtime.AuthorizerFunc(
		func(r *http.Request, p interface{}) error {
			principal, success := p.(*models.Principal)
			if !success {
				d.logger.Error().Msg("failed to type assert principal")
				return ErrAuthZInternal
			}
			d.logger.Debug().
				Str("user", principal.Subject).
				Msg("Authorize checking perms")
			ctx := r.Context()

			nor, err := findNOR(r, mc)
			if err != nil {
				d.logger.Err(err).Msg("Authorize could not find NOR")
				return ErrAuthZInternal
			}

			var authorized bool
			switch nor.NameSpace {
			case NamespaceSessions:
				authorized, err = d.AuthorizeSessions(ctx, r, mc, nor, principal)
				if err != nil {
					d.logger.Err(err).Msg("error while checking sessions permissions")
					return ErrAuthZInternal
				}
			case NamespaceUsers:
				authorized, err = d.AuthorizeUserProfiles(ctx, nor, principal)
				if err != nil {
					d.logger.Err(err).Msg("error while checking users permissions")
					return ErrAuthZInternal
				}
			case NamespaceRaces:
				authorized, err = d.AuthorizeRaces(ctx, r, mc, nor, principal)
				if err != nil {
					d.logger.Err(err).Msg("error while checking races permissions")
					return ErrAuthZInternal
				}
			case NamespaceInvitations:
				authorized, err = d.AuthorizeInvitations(ctx, r, mc, nor, principal)
				if err != nil {
					d.logger.Err(err).Msg("error while checking invitations permissions")
					return ErrAuthZInternal
				}

			case NamespaceToken:
				authorized = true
			}

			d.logger.Debug().
				Bool("authorized", authorized).
				Str("user", principal.NickName).
				Str("user id", principal.Subject).
				Msg("Authorize decision")

			if !authorized {
				return ErrAuthZFailed
			}

			return nil
		},
	)
}

func (d *Authorizer) AuthorizeRaces(ctx context.Context, r *http.Request, mc *middleware.Context, nor *NOR, principal *models.Principal) (bool, error) {
	// global manager bypasses all computations
	if principal.IsGlobalManager {
		return true, nil
	}

	// here we assume the route will always match because if it did not match it should never have come this far
	route, _, _ := mc.RouteInfo(r)
	var params middleware.RequestBinder
	var askedUserProfile string

	// get the params according to the handler
	switch route.Handler.(type) {
	case operations.RacesListHandler:
		p := operations.NewRacesListParams()
		params = &p
		if err := mc.BindValidRequest(r, route, params); err != nil { // bind params
			return false, err
		}
		// extract the asked user profile from the params
		askedUserProfile = p.UserProfileID
	case operations.RaceCreateHandler:
		p := operations.NewRaceCreateParams()
		params := &p
		if err := mc.BindValidRequest(r, route, params); err != nil { // bind params
			return false, err
		}
		askedUserProfile = p.UserProfileID
	case operations.RaceReadHandler:
		p := operations.NewRaceReadParams()
		params := &p
		if err := mc.BindValidRequest(r, route, params); err != nil { // bind params
			return false, err
		}
		askedUserProfile = p.UserProfileID

	default:
		d.logger.Error().Msg("authorize races type switch failed to determine handler for request")
		return false, errors.New("failed to determine handler for request")
	}

	if principal.Subject == askedUserProfile {
		d.logger.Debug().
			Str("userID", principal.Subject).
			Msg("authorizing user to list own races")
		return true, nil
	}
	d.logger.Debug().
		Str("actual userID", principal.Subject).
		Str("asked userID", askedUserProfile).
		Msg("user wanted to read someone else's races --> refused")
	return false, nil
}

func (d *Authorizer) AuthorizeUserProfiles(ctx context.Context, nor *NOR, principal *models.Principal) (bool, error) {
	// global manager always has access to all user profiles API
	if principal.IsGlobalManager {
		return true, nil
	}

	// user wants to interact with own profile...
	if nor.Object == principal.Subject {
		switch nor.Relation {
		// she is then authorized to read, delete and update
		case RelationRead, RelationDelete, RelationUpdate:
			return true, nil
		default:
			return false, nil
		}
	}
	// any other case is forbidden
	return false, nil
}

type SessionPrivateQueryResult struct {
	Private bool `db:"private"`
}

func (d *Authorizer) AuthorizeSessions(ctx context.Context, r *http.Request, mc *middleware.Context, nor *NOR, principal *models.Principal) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}

	query := database.SQ.
		Select().
		Columns(
			models.UserProfileSessionRelDBUserProfileIDColumn,
			models.UserProfileSessionRelDBSessionIDColumn,
			models.UserProfileSessionRelDBIsManagerColumn,
		).
		From(models.UserProfileSessionRelDBTable).
		Where(
			sq.And{
				sq.Eq{models.UserProfileSessionRelDBUserProfileIDColumn: principal.Subject},
				sq.Eq{models.UserProfileSessionRelDBSessionIDColumn: nor.Object},
			},
		)

	switch nor.Relation {
	case RelationCreate, RelationUpdate, RelationDelete:
		var authRes models.UserProfileSessionRelDB
		if err := database.GetContext(ctx, d.db, &authRes, query, &d.logger); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				log.Debug().Msg("auth: no matching userprofile session relation --> reject")
				return false, nil
			}
			return false, err
		}
		return authRes.IsManager, nil
	case RelationRead:
		route, _, _ := mc.RouteInfo(r)
		var params middleware.RequestBinder
		var askedSessionID string

		// get the params according to the handler
		switch route.Handler.(type) {
		// in a relation read we should be on the session read handler
		case operations.SessionReadHandler:
			p := operations.NewSessionReadParams()
			params = &p
			if err := mc.BindValidRequest(r, route, params); err != nil { // bind params
				return false, err
			}
			// extract the asked user profile from the params
			askedSessionID = p.SessionID
		default:
			panic("route handler type should have been SessionReadHandler. Check you did not assign a read relation to a session on some other handler")
		}

		query := database.SQ.
			Select().
			Columns(
				models.SessionDBPrivateColumn,
			).
			From(models.SessionDBTable).
			Where(sq.Eq{models.SessionDBIDColumn: askedSessionID})

		var p SessionPrivateQueryResult
		if err := database.GetContext(ctx, d.db, &p, query, &d.logger); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				log.Debug().Msg("auth: no matching session found --> reject")
				return false, nil
			}
			return false, err
		}

		// if session is public no problem
		if !p.Private {
			return true, nil
		}

		// if session is private only members have access
		var authRes models.UserProfileSessionRelDB
		if err := database.GetContext(ctx, d.db, &authRes, query, &d.logger); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				log.Debug().Msg("auth: no matching userprofile session relation for a private session --> reject")
				return false, nil
			}
			return false, err
		}

		// if we are here we are at least a member of the session, so we are authorized to read the session content
		return true, nil

	default:
		return false, nil
	}
}

func (d *Authorizer) AuthorizeInvitations(ctx context.Context, r *http.Request, mc *middleware.Context, nor *NOR, principal *models.Principal) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}

	query := database.SQ.
		Select().
		Columns(
			models.UserProfileSessionRelDBUserProfileIDColumn,
			models.UserProfileSessionRelDBSessionIDColumn,
			models.UserProfileSessionRelDBIsManagerColumn,
		).
		From(models.UserProfileSessionRelDBTable).
		Where(
			sq.And{
				sq.Eq{models.UserProfileSessionRelDBUserProfileIDColumn: principal.Subject},
				sq.Eq{models.UserProfileSessionRelDBSessionIDColumn: nor.Object},
			},
		)

	switch nor.Relation {
	case RelationCreate, RelationUpdate, RelationDelete:
		var authRes models.UserProfileSessionRelDB
		if err := database.GetContext(ctx, d.db, &authRes, query, &d.logger); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				log.Debug().Msg("auth: no matching userprofile session relation --> reject")
				return false, nil
			}
			return false, err
		}
		return authRes.IsManager, nil
	case RelationRead:
		route, _, _ := mc.RouteInfo(r)
		var params middleware.RequestBinder
		var askedSessionID string

		// get the params according to the handler
		switch route.Handler.(type) {
		// in a relation read we should be on the session read handler
		case operations.SessionReadHandler:
			p := operations.NewSessionReadParams()
			params = &p
			if err := mc.BindValidRequest(r, route, params); err != nil { // bind params
				return false, err
			}
			// extract the asked user profile from the params
			askedSessionID = p.SessionID
		default:
			panic("route handler type should have been SessionReadHandler. Check you did not assign a read relation to a session on some other handler")
		}

		query := database.SQ.
			Select().
			Columns(
				models.SessionDBPrivateColumn,
			).
			From(models.SessionDBTable).
			Where(sq.Eq{models.SessionDBIDColumn: askedSessionID})

		var p SessionPrivateQueryResult
		if err := database.GetContext(ctx, d.db, &p, query, &d.logger); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				log.Debug().Msg("auth: no matching session found --> reject")
				return false, nil
			}
			return false, err
		}

		// if session is public no problem
		if !p.Private {
			return true, nil
		}

		// if session is private only members have access
		var authRes models.UserProfileSessionRelDB
		if err := database.GetContext(ctx, d.db, &authRes, query, &d.logger); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				log.Debug().Msg("auth: no matching userprofile session relation for a private session --> reject")
				return false, nil
			}
			return false, err
		}

		// if we are here we are at least a member of the session, so we are authorized to read the session content
		return true, nil

	default:
		return false, nil
	}
}

type ErrInvalidRoute struct{}

func (e *ErrInvalidRoute) Error() string { return "invalid route" }

// ErrOperationNotSupported ...
type ErrOperationNotSupported struct {
	Key  string
	Name string
}

// Error ...
func (e *ErrOperationNotSupported) Error() string {
	return fmt.Sprintf("operation %q not supported, please add an %s in your swagger", e.Name, e.Key)
}

// NOR NameSpace Object Relation
type NOR struct {
	NameSpace string
	Object    string
	Relation  string
}

// findNOR returns an NameSpace Object Relation object (NOR) for the current request
func findNOR(r *http.Request, mc *middleware.Context) (*NOR, error) {
	mr, _, matched := mc.RouteInfo(r)
	if !matched {
		return nil, &ErrInvalidRoute{}
	}
	operationID := mr.Operation.ID
	ns, found := mr.Operation.Extensions.GetString(XAuthNamespace)
	if !found {
		return nil, &ErrOperationNotSupported{Name: operationID, Key: XAuthNamespace}
	}
	rel, found := mr.Operation.Extensions.GetString(XAuthRelation)
	if !found {
		return nil, &ErrOperationNotSupported{Name: operationID, Key: XAuthRelation}
	}

	// strip the basepath from the object search
	cutLen := len(mr.BasePath)
	return &NOR{NameSpace: ns, Object: r.URL.Path[cutLen:], Relation: rel}, nil
}
