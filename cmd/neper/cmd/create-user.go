package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/m4rw3r/uuid"
	orusapi "orus.io/orus-io/go-orusapi"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/lib/serial"
	"github.com/neper-stars/neper/models"
)

// CreateUserCmd is the command to create a new user
type CreateUserCmd struct {
	db  *database.Options
	log *orusapi.LoggingOptions

	Args struct {
		Username string `positional-arg-name:"username" required:"yes" description:"the username (nickname) for the new user"`
	} `positional-args:"yes"`

	Email     string `long:"email" description:"email address for the user" default:""`
	IsManager bool   `long:"manager" description:"make this user a manager"`
}

// NewCreateUserCmd creates a new CreateUserCmd
func NewCreateUserCmd(dbOptions *database.Options, loggingOptions *orusapi.LoggingOptions) *CreateUserCmd {
	return &CreateUserCmd{
		db:  dbOptions,
		log: loggingOptions,
	}
}

// Execute creates a new user in the database
func (cmd *CreateUserCmd) Execute([]string) error {
	log := cmd.log.Logger()
	ctx := context.Background()

	// Generate a random API key (32 bytes = 64 hex characters)
	apiKey := make([]byte, 32)
	if _, err := rand.Read(apiKey); err != nil {
		return fmt.Errorf("failed to generate API key: %w", err)
	}
	apiKeyHex := hex.EncodeToString(apiKey)

	// Generate a UUID for the user ID
	userID, err := uuid.V4()
	if err != nil {
		return fmt.Errorf("failed to generate user ID: %w", err)
	}

	// Connect to database
	db, err := cmd.db.Open()
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Err(err).Msg("failed to close database connection")
		}
	}()

	// Set email to username if not provided
	email := cmd.Email
	if email == "" {
		email = cmd.Args.Username + "@neper.local"
	}

	// Create the user profile using the model
	userProfileDB := models.UserProfileDB{
		UserProfile: models.UserProfile{
			ID:        userID.String(),
			Email:     email,
			State:     models.UserProfileStateActive,
			IsManager: cmd.IsManager,
			Nickname:  cmd.Args.Username,
		},
		APIKey: apiKeyHex,
	}

	// Insert the user using SQLHelper
	tx, err := database.Begin(ctx, db)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.RollbackIfOpened(*log)

	sqlH := database.NewSQLHelper(ctx, tx, *log)
	if _, err := sqlH.Insert(&userProfileDB); err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Assign a serial key to the user
	serialKey, err := serial.AssignKeyToUser(ctx, db, userID.String())
	if err != nil {
		log.Warn().Err(err).Msg("failed to assign serial key (serials may not be loaded yet)")
	}

	log.Info().
		Str("username", cmd.Args.Username).
		Str("id", userID.String()).
		Bool("is_manager", cmd.IsManager).
		Str("serial_key", serialKey).
		Msg("user created successfully")

	fmt.Println("API Key:", apiKeyHex)
	if serialKey != "" {
		fmt.Println("Serial Key:", serialKey)
	}

	return nil
}

func init() {
	cmd := NewCreateUserCmd(DatabaseOptions, LoggingOptions)

	_, err := parser.AddCommand("create-user", "Create a new user with a random API key", "", cmd)
	if err != nil {
		Logger.Fatal().Msg(err.Error())
	}
}
