package admin

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/m4rw3r/uuid"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/lib/serial"
	"github.com/neper-stars/neper/models"
)

// EnsureAdminUser checks if an admin user exists and creates one if not.
// This is idempotent - safe to call on every startup.
// Returns the API key if a new user was created, empty string if user already exists.
func EnsureAdminUser(ctx context.Context, db *sqlx.DB, log *zerolog.Logger, opts *Options) (string, error) {
	if !opts.Enabled() {
		return "", nil
	}

	sqlH := database.NewSQLHelper(ctx, db, *log)

	// Check if user already exists by nickname
	var existingUser models.UserProfileDB
	err := sqlH.GetWhere(&existingUser, map[string]interface{}{
		models.UserProfileDBNicknameColumn: opts.AdminUsername,
	})

	if err == nil {
		// User already exists
		log.Info().
			Str("username", opts.AdminUsername).
			Str("id", existingUser.ID).
			Msg("admin user already exists, skipping creation")
		return "", nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		// Unexpected error
		return "", err
	}

	// User doesn't exist, create it
	apiKey := opts.AdminAPIKey
	if apiKey == "" {
		// Generate a random API key (32 bytes = 64 hex characters)
		apiKeyBytes := make([]byte, 32)
		if _, err := rand.Read(apiKeyBytes); err != nil {
			return "", err
		}
		apiKey = hex.EncodeToString(apiKeyBytes)
	}

	// Generate a UUID for the user ID
	userID, err := uuid.V4()
	if err != nil {
		return "", err
	}

	// Set email if not provided
	email := opts.AdminEmail
	if email == "" {
		email = opts.AdminUsername + "@neper.local"
	}

	// Create the user profile
	userProfileDB := models.UserProfileDB{
		UserProfile: models.UserProfile{
			ID:        userID.String(),
			Email:     email,
			State:     models.UserProfileStateActive,
			IsManager: true, // Admin user is always a manager
			Nickname:  opts.AdminUsername,
		},
		APIKey: apiKey,
	}

	// Insert the user using a transaction
	tx, err := database.Begin(ctx, db)
	if err != nil {
		return "", err
	}
	defer tx.RollbackIfOpened(*log)

	txSqlH := database.NewSQLHelper(ctx, tx, *log)
	if _, err := txSqlH.Insert(&userProfileDB); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	// Assign a serial key to the user (non-fatal if it fails)
	serialKey, err := serial.AssignKeyToUser(ctx, db, userID.String())
	if err != nil {
		log.Warn().Err(err).Msg("failed to assign serial key to admin user")
	}

	log.Info().
		Str("username", opts.AdminUsername).
		Str("id", userID.String()).
		Str("email", email).
		Str("serial_key", serialKey).
		Msg("admin user created successfully")

	return apiKey, nil
}
