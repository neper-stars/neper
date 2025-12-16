package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/m4rw3r/uuid"
	orusapi "orus.io/orus-io/go-orusapi"
	"orus.io/orus-io/go-orusapi/database"
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
	defer db.Close()

	// Set email to username if not provided
	email := cmd.Email
	if email == "" {
		email = cmd.Args.Username + "@neper.local"
	}

	// Insert the user
	_, err = db.Exec(`
		INSERT INTO user_profile (id, email, is_active, is_manager, nickname, apikey)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, userID.String(), email, true, cmd.IsManager, cmd.Args.Username, apiKeyHex)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	log.Info().
		Str("username", cmd.Args.Username).
		Str("id", userID.String()).
		Bool("is_manager", cmd.IsManager).
		Msg("user created successfully")

	fmt.Println("API Key:", apiKeyHex)

	return nil
}

func init() {
	cmd := NewCreateUserCmd(DatabaseOptions, LoggingOptions)

	_, err := parser.AddCommand("create-user", "Create a new user with a random API key", "", cmd)
	if err != nil {
		Logger.Fatal().Msg(err.Error())
	}
}
