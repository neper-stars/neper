// This file is generated only once and is safe to edit

package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/go-openapi/swag"
	"github.com/nats-io/nats.go"

	"github.com/neper-stars/neper/auth"
	"github.com/neper-stars/neper/lib/admin"
	"github.com/neper-stars/neper/lib/embeddednats"
	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/lib/race"
	"github.com/neper-stars/neper/lib/racefiles"
	"github.com/neper-stars/neper/lib/registration"
	"github.com/neper-stars/neper/lib/serial"
	"github.com/neper-stars/neper/lib/session"
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/restapi"
)

var (
	// TokenOptions is the authentication token options
	TokenOptions        = auth.NewTokenOptions()
	StarsRunnerOptions  = stars.NewRunnerOptions()
	EmbeddedNatsOptions = embeddednats.NewEmbeddedNatsOptions()
	NatsClientOptions   = embeddednats.NewNatsClientOptions()
	RegistrationOptions = registration.NewOptions()
	RaceOptions         = race.NewOptions()
	SessionOptions      = session.NewOptions()
	AdminOptions        = admin.NewOptions()
)

func setupServerCmd(cmd *ServeCmd) {
	cmd.API.CommandLineOptionsGroups = append(
		cmd.API.CommandLineOptionsGroups,
		swag.CommandLineOptionsGroup{
			ShortDescription: "auth",
			LongDescription:  "authentication settings",
			Options:          TokenOptions,
		},
		swag.CommandLineOptionsGroup{
			ShortDescription: "runner",
			LongDescription:  "stars runner",
			Options:          StarsRunnerOptions,
		},
		swag.CommandLineOptionsGroup{
			ShortDescription: "nats-server",
			LongDescription:  "nats server config",
			Options:          EmbeddedNatsOptions,
		},
		swag.CommandLineOptionsGroup{
			ShortDescription: "nats-client",
			LongDescription:  "nats client config",
			Options:          NatsClientOptions,
		},
		swag.CommandLineOptionsGroup{
			ShortDescription: "registration",
			LongDescription:  "registration settings",
			Options:          RegistrationOptions,
		},
		swag.CommandLineOptionsGroup{
			ShortDescription: "race",
			LongDescription:  "race upload settings",
			Options:          RaceOptions,
		},
		swag.CommandLineOptionsGroup{
			ShortDescription: "session",
			LongDescription:  "session settings",
			Options:          SessionOptions,
		},
		swag.CommandLineOptionsGroup{
			ShortDescription: "admin",
			LongDescription:  "admin user auto-creation settings",
			Options:          AdminOptions,
		},
	)
}

func setupServeConfig(config *restapi.Config) error {
	// Rebuild the logger from parsed options (updates the package-level Logger variable)
	Logger = *LoggingOptions.Logger()
	config.Log = Logger

	// Channel to signal when heavy initialization is complete
	// Game endpoints should wait for this before processing requests
	config.InitDone = make(chan struct{})

	// Run heavy initialization tasks in background so server can start immediately
	// This allows health checks to pass while initialization is in progress
	// Tasks run sequentially to avoid overloading the machine
	go func() {
		defer close(config.InitDone)
		log := config.Log.With().Str("component", "background-init").Logger()

		// Load serial keys into database if not already loaded
		// This is idempotent and safe to call on every startup
		if err := serial.LoadKeysIntoDB(context.Background(), config.DB, &log); err != nil {
			log.Err(err).Msg("failed to load serial keys")
		}

		// Auto-create admin user if configured
		// This is idempotent - safe to call on every startup
		if AdminOptions.Enabled() {
			apiKey, err := admin.EnsureAdminUser(context.Background(), config.DB, &log, AdminOptions)
			if err != nil {
				log.Err(err).Msg("failed to ensure admin user")
			} else if apiKey != "" {
				log.Info().
					Str("username", AdminOptions.AdminUsername).
					Str("apikey", apiKey).
					Msg("new admin user created - save the API key!")
			}
		}

		// Initialize wine prefix AFTER serial keys (sequential to avoid overloading)
		log.Info().Msg("initializing wine prefix...")
		if config.StarsRunner != nil {
			if err := config.StarsRunner.Init(); err != nil {
				log.Err(err).Msg("failed to initialize stars runner")
			} else {
				log.Info().Bool("xvfb_running", config.StarsRunner.IsXvfbRunning()).Msg("stars runner initialized")
			}
		}

		log.Info().Msg("background initialization complete")
	}()

	config.BaseURL = InfoOptions.BaseURL
	// This is where the api config can be customized at will
	config.TokenOptions = *TokenOptions

	// Set default Now function if not provided (tests can override this)
	if config.Now == nil {
		config.Now = time.Now
	}

	// Initialize the authenticator
	config.Authenticator = auth.NewAuth(config.TokenOptions, config.DB, config.Now, config.Log)

	// Initialize registration options and rate limiter
	config.RegistrationOptions = RegistrationOptions
	config.RegistrationLimiter = registration.NewRateLimiter(RegistrationOptions)

	// Initialize race options
	config.RaceOptions = RaceOptions

	// Initialize session options
	config.SessionOptions = SessionOptions

	runner, err := stars.NewRunner(&config.Log, StarsRunnerOptions)
	if err != nil {
		return err
	}
	config.StarsRunner = runner
	// Note: StarsRunner.Init() is called in background goroutine to avoid blocking startup

	// Initialize the race file processor with options from runner settings
	config.RaceProcessor = racefiles.NewProcessor(racefiles.ProcessorOptions{
		StripPassword: StarsRunnerOptions.StripRacePasswords,
		FixCorrupted:  StarsRunnerOptions.FixRaceFiles,
	})

	// ----------------------------
	// NATS configuration
	// ----------------------------
	config.NatsClientOptions = NatsClientOptions
	privKey, err := config.NatsClientOptions.NPrivateKey(config.Log)
	if err != nil {
		return err
	}
	signatureHandler, err := embeddednats.NewClientSigHandler(privKey)
	if err != nil {
		return err
	}

	config.EmbeddedNatsOptions = *EmbeddedNatsOptions
	if !config.EmbeddedNatsOptions.NoNatsServer {
		ns := embeddednats.NewEmbeddedServer(signatureHandler.PubKey())
		config.NatsServer = ns
		go ns.Run()
		// Wait for NATS server to be ready before proceeding
		if !ns.WaitReady(10 * time.Second) {
			return fmt.Errorf("NATS server failed to start within timeout")
		}
	}

	connHandlers := embeddednats.NewConnHandlers(&config.Log)

	cOpts := nats.Options{
		Url:            config.NatsClientOptions.Url,
		AllowReconnect: false, // TODO: understand how to make this work... setting yes here no connection goes up...
		MaxReconnect:   60,
		// ClosedCB:             nil,
		DisconnectedErrCB: connHandlers.DisconnectErrHandler,
		ConnectedCB:       connHandlers.ConnHandler,
		// ReconnectedCB:        nil,
		// DiscoveredServersCB:  nil,
		AsyncErrorCB:         connHandlers.ErrHandler,
		Nkey:                 signatureHandler.PubKey(),
		SignatureCB:          signatureHandler.Sign,
		RetryOnFailedConnect: true,
	}

	// connect the client
	config.Log.Info().Msg("starting the NATS client")
	nc, err := cOpts.Connect()
	if err != nil {
		return err
	}
	// config.Log.Info().Str("natsConnStatus", nc.Status().String()).Msg("nats status")
	config.NatsClientConn = nc

	// Initialize the notification service
	config.NotifyService = notify.NewService(nc, &config.Log)
	// NATS out ------------------------------------

	// start the turn generator
	tg := stars.NewTurnGenerator(&config.Log, config.NatsClientConn, config.DB, runner, config.NotifyService)
	config.TurnGenerator = tg
	go config.TurnGenerator.Run(context.Background())
	// wait for the turn generator to be fully initialized
	<-config.TurnGenerator.Ready()
	if !config.TurnGenerator.IsRunning() {
		return fmt.Errorf("TurnGenerator failed to start")
	}

	return nil
}
