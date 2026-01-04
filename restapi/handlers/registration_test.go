package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/registration"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

func TestRegisterHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	// Enable registration for tests (disabled by default)
	opts := registration.NewOptions()
	opts.Enabled = true
	registerHandler := NewRegisterHandler(&log, testdb.DB, opts, nil, nil)

	t.Run("successful_registration", func(t *testing.T) {
		params := operations.RegisterParams{
			Registration: &models.RegistrationRequest{
				Nickname: "newuser",
				Email:    "newuser@example.com",
				Message:  ptrString("I want to join to play Stars!"),
			},
		}

		result, err := registerHandler.handle(ctx, params)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "newuser", result.Nickname)
		require.NotEmpty(t, result.UserID)
		require.NotEmpty(t, result.Apikey, "should have generated an API key")

		// Verify user state in the database
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var userDB models.UserProfileDB
		err = sqlH.GetByPKey(&userDB, result.UserID)
		require.NoError(t, err)
		require.Equal(t, "newuser@example.com", userDB.Email)
		require.True(t, userDB.Pending, "new user should be pending")
		require.True(t, userDB.IsActive, "new user should be active for authentication")
		require.NotNil(t, userDB.RegistrationMessage)
		require.Equal(t, "I want to join to play Stars!", *userDB.RegistrationMessage)
		require.Equal(t, result.Apikey, userDB.APIKey, "API key should match")
		require.True(t, result.Pending, "result should indicate pending status")
	})

	t.Run("duplicate_nickname_fails", func(t *testing.T) {
		params := operations.RegisterParams{
			Registration: &models.RegistrationRequest{
				Nickname: "newuser", // same nickname as above
				Email:    "different@example.com",
			},
		}

		_, err := registerHandler.handle(ctx, params)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrConflict), "should get conflict error for duplicate nickname")
	})

	t.Run("duplicate_email_fails", func(t *testing.T) {
		params := operations.RegisterParams{
			Registration: &models.RegistrationRequest{
				Nickname: "anotheruser",
				Email:    "newuser@example.com", // same email as first registration
			},
		}

		_, err := registerHandler.handle(ctx, params)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrConflict), "should get conflict error for duplicate email")
	})

	t.Run("empty_nickname_fails", func(t *testing.T) {
		params := operations.RegisterParams{
			Registration: &models.RegistrationRequest{
				Nickname: "",
				Email:    "valid@example.com",
			},
		}

		_, err := registerHandler.handle(ctx, params)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrInvalid), "should get invalid error for empty nickname")
	})

	t.Run("empty_email_fails", func(t *testing.T) {
		params := operations.RegisterParams{
			Registration: &models.RegistrationRequest{
				Nickname: "validnick",
				Email:    "",
			},
		}

		_, err := registerHandler.handle(ctx, params)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrInvalid), "should get invalid error for empty email")
	})
}

func TestRegisterHandler_NoApprovalNeeded(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	// Enable registration with NeedApproval=false
	opts := registration.NewOptions()
	opts.Enabled = true
	opts.NeedApproval = false
	registerHandler := NewRegisterHandler(&log, testdb.DB, opts, nil, nil)

	t.Run("registration_without_approval", func(t *testing.T) {
		params := operations.RegisterParams{
			Registration: &models.RegistrationRequest{
				Nickname: "directuser",
				Email:    "directuser@example.com",
			},
		}

		result, err := registerHandler.handle(ctx, params)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "directuser", result.Nickname)
		require.NotEmpty(t, result.Apikey)
		require.False(t, result.Pending, "result should indicate not pending")

		// Verify user state in the database
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var userDB models.UserProfileDB
		err = sqlH.GetByPKey(&userDB, result.UserID)
		require.NoError(t, err)
		require.False(t, userDB.Pending, "user should not be pending")
		require.True(t, userDB.IsActive, "user should be active")
	})
}

func TestPendingRegistrationListHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	opts := registration.NewOptions()
	opts.Enabled = true
	registerHandler := NewRegisterHandler(&log, testdb.DB, opts, nil, nil)
	listHandler := NewPendingRegistrationListHandler(&log, testdb.DB)

	// Create some pending registrations
	for i := 0; i < 3; i++ {
		params := operations.RegisterParams{
			Registration: &models.RegistrationRequest{
				Nickname: "pendinguser" + string(rune('0'+i)),
				Email:    "pending" + string(rune('0'+i)) + "@example.com",
			},
		}
		_, err := registerHandler.handle(ctx, params)
		require.NoError(t, err)
	}

	t.Run("global_manager_can_list_pending", func(t *testing.T) {
		adminPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "adminID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}

		params := operations.PendingRegistrationListParams{}
		users, err := listHandler.handle(ctx, params, adminPrincipal)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(users), 3, "should have at least 3 pending users")

		for _, user := range users {
			require.True(t, user.Pending, "all listed users should be pending")
		}
	})

	t.Run("regular_user_cannot_list_pending", func(t *testing.T) {
		regularPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "regularID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		params := operations.PendingRegistrationListParams{}
		_, err := listHandler.handle(ctx, params, regularPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden))
	})
}

func TestPendingRegistrationApproveHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	opts := registration.NewOptions()
	opts.Enabled = true
	registerHandler := NewRegisterHandler(&log, testdb.DB, opts, nil, nil)
	approveHandler := NewPendingRegistrationApproveHandler(&log, testdb.DB, nil)

	// Create a pending registration
	regParams := operations.RegisterParams{
		Registration: &models.RegistrationRequest{
			Nickname: "toapprove",
			Email:    "toapprove@example.com",
		},
	}
	pendingUser, err := registerHandler.handle(ctx, regParams)
	require.NoError(t, err)

	t.Run("global_manager_can_approve", func(t *testing.T) {
		adminPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "adminID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}

		params := operations.PendingRegistrationApproveParams{
			UserProfileID: pendingUser.UserID,
		}
		result, err := approveHandler.handle(ctx, params, adminPrincipal)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, pendingUser.UserID, result.UserID)
		require.NotEmpty(t, result.Apikey, "should return the API key")

		// Verify the user is now approved in the database
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var userDB models.UserProfileDB
		err = sqlH.GetByPKey(&userDB, pendingUser.UserID)
		require.NoError(t, err)
		require.False(t, userDB.Pending, "user should no longer be pending")
		require.True(t, userDB.IsActive, "user should be active")
		require.NotEmpty(t, userDB.APIKey, "user should have an API key")
	})

	t.Run("approving_non_pending_user_fails", func(t *testing.T) {
		adminPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "adminID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}

		// Try to approve the same user again
		params := operations.PendingRegistrationApproveParams{
			UserProfileID: pendingUser.UserID,
		}
		_, err := approveHandler.handle(ctx, params, adminPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrInvalid), "should get invalid error for non-pending user")
	})

	t.Run("regular_user_cannot_approve", func(t *testing.T) {
		// Create another pending user
		regParams := operations.RegisterParams{
			Registration: &models.RegistrationRequest{
				Nickname: "anotherpending",
				Email:    "anotherpending@example.com",
			},
		}
		anotherPending, err := registerHandler.handle(ctx, regParams)
		require.NoError(t, err)

		regularPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "regularID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		params := operations.PendingRegistrationApproveParams{
			UserProfileID: anotherPending.UserID,
		}
		_, err = approveHandler.handle(ctx, params, regularPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden))
	})
}

func TestPendingRegistrationRejectHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	opts := registration.NewOptions()
	opts.Enabled = true
	registerHandler := NewRegisterHandler(&log, testdb.DB, opts, nil, nil)
	rejectHandler := NewPendingRegistrationRejectHandler(&log, testdb.DB, nil)

	t.Run("global_manager_can_reject", func(t *testing.T) {
		// Create a pending registration
		regParams := operations.RegisterParams{
			Registration: &models.RegistrationRequest{
				Nickname: "toreject",
				Email:    "toreject@example.com",
			},
		}
		pendingUser, err := registerHandler.handle(ctx, regParams)
		require.NoError(t, err)

		adminPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "adminID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}

		params := operations.PendingRegistrationRejectParams{
			UserProfileID: pendingUser.UserID,
		}
		err = rejectHandler.handle(ctx, params, adminPrincipal)
		require.NoError(t, err)

		// Verify the user is deleted
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var userDB models.UserProfileDB
		err = sqlH.GetByPKey(&userDB, pendingUser.UserID)
		require.Error(t, err, "user should be deleted after rejection")
	})

	t.Run("regular_user_cannot_reject", func(t *testing.T) {
		// Create a pending registration
		regParams := operations.RegisterParams{
			Registration: &models.RegistrationRequest{
				Nickname: "toreject2",
				Email:    "toreject2@example.com",
			},
		}
		pendingUser, err := registerHandler.handle(ctx, regParams)
		require.NoError(t, err)

		regularPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "regularID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		params := operations.PendingRegistrationRejectParams{
			UserProfileID: pendingUser.UserID,
		}
		err = rejectHandler.handle(ctx, params, regularPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden))
	})
}

func TestRegisterHandler_Disabled(t *testing.T) {
	log := testutils.GetLogger(t)
	testdb := database.GetTestDB(log.WithContext(context.Background()), t, migration.Source)
	defer testdb.Close()

	// Create handler with default options (registration disabled by default)
	opts := registration.NewOptions()
	registerHandler := NewRegisterHandler(&log, testdb.DB, opts, nil, nil)

	t.Run("registration_disabled_returns_503", func(t *testing.T) {
		params := operations.RegisterParams{
			HTTPRequest: &http.Request{},
			Registration: &models.RegistrationRequest{
				Nickname: "testuser",
				Email:    "testuser@example.com",
			},
		}

		resp := registerHandler.Handle(params)
		require.IsType(t, &operations.RegisterServiceUnavailable{}, resp)
	})
}

func TestRegisterHandler_RateLimiting(t *testing.T) {
	log := testutils.GetLogger(t)
	testdb := database.GetTestDB(log.WithContext(context.Background()), t, migration.Source)
	defer testdb.Close()

	// Create handler with very restrictive rate limiting (1 per minute, burst of 2)
	opts := &registration.Options{
		Enabled:            true,
		RateLimitPerMinute: 1,
		RateLimitBurst:     2,
	}
	limiter := registration.NewRateLimiter(opts)
	defer limiter.Close()

	registerHandler := NewRegisterHandler(&log, testdb.DB, opts, limiter, nil)

	t.Run("rate_limiting_returns_429", func(t *testing.T) {
		// Make requests until we hit the rate limit
		for i := 0; i < 5; i++ {
			params := operations.RegisterParams{
				HTTPRequest: &http.Request{
					RemoteAddr: "192.168.1.100:12345",
				},
				Registration: &models.RegistrationRequest{
					Nickname: "ratelimituser" + string(rune('0'+i)),
					Email:    "ratelimit" + string(rune('0'+i)) + "@example.com",
				},
			}

			resp := registerHandler.Handle(params)

			// First few requests should succeed (burst allows it)
			// After that, we should get rate limited
			if i < 2 {
				require.IsType(t, &operations.RegisterCreated{}, resp, "request %d should succeed (within burst)", i)
			} else {
				require.IsType(t, &operations.RegisterTooManyRequests{}, resp, "request %d should be rate limited", i)
			}
		}
	})
}

func ptrString(s string) *string {
	return &s
}
