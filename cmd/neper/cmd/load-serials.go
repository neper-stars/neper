package cmd

import (
	"context"

	orusapi "orus.io/orus-io/go-orusapi"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/lib/serial"
)

// LoadSerialsCmd is the command to load serial keys into the database
type LoadSerialsCmd struct {
	db  *database.Options
	log *orusapi.LoggingOptions
}

// NewLoadSerialsCmd creates a new LoadSerialsCmd
func NewLoadSerialsCmd(dbOptions *database.Options, loggingOptions *orusapi.LoggingOptions) *LoadSerialsCmd {
	return &LoadSerialsCmd{
		db:  dbOptions,
		log: loggingOptions,
	}
}

// Execute loads all serial keys into the database
func (cmd *LoadSerialsCmd) Execute([]string) error {
	log := cmd.log.Logger()
	ctx := context.Background()

	log.Info().Msg("connecting to database...")

	db, err := cmd.db.Open()
	if err != nil {
		log.Err(err).Msg("failed to connect to database")
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Err(err).Msg("failed to close database connection")
		}
	}()

	log.Info().Msg("loading serial keys into database...")

	if err := serial.LoadKeysIntoDB(ctx, db, log); err != nil {
		log.Err(err).Msg("failed to load serial keys")
		return err
	}

	// Show stats
	available, err := serial.GetAvailableKeyCount(ctx, db)
	if err != nil {
		log.Err(err).Msg("failed to get available key count")
		return err
	}

	assigned, err := serial.GetAssignedKeyCount(ctx, db)
	if err != nil {
		log.Err(err).Msg("failed to get assigned key count")
		return err
	}

	log.Info().
		Int("available", available).
		Int("assigned", assigned).
		Int("total", available+assigned).
		Msg("serial key statistics")

	return nil
}

func init() {
	cmd := NewLoadSerialsCmd(DatabaseOptions, LoggingOptions)

	_, err := parser.AddCommand("load-serials", "Load serial keys into database", "Load all serial keys from the embedded file into the database. This should be run once after the first migration.", cmd)
	if err != nil {
		Logger.Fatal().Msg(err.Error())
	}
}
