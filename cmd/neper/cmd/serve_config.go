// This file is generated only once and is safe to edit

package cmd

import (
	"github.com/go-openapi/swag"
	"github.com/nats-io/nats.go"

	"context"

	"github.com/neper-stars/neper/auth"
	"github.com/neper-stars/neper/lib/embeddednats"
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/restapi"
)

var (
	// TokenOptions is the authentication token options
	TokenOptions        = auth.NewTokenOptions()
	StarsRunnerOptions  = stars.NewRunnerOptions()
	EmbeddedNatsOptions = embeddednats.NewEmbeddedNatsOptions()
	NatsClientOptions   = embeddednats.NewNatsClientOptions()
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
	)
}

func setupServeConfig(config *restapi.Config) error {
	config.BaseURL = InfoOptions.BaseURL
	// This is where the api config can be customized at will
	config.TokenOptions = *TokenOptions

	// Initialize the authenticator
	config.Authenticator = auth.NewAuth(config.TokenOptions, config.DB, config.Now, config.Log)

	runner, err := stars.NewRunner(&config.Log, StarsRunnerOptions)
	if err != nil {
		return err
	}
	config.StarsRunner = runner
	if err := config.StarsRunner.Init(); err != nil {
		return err
	}

	// ----------------------------
	// NATS configuration
	// ----------------------------
	config.NatsClientOptions = NatsClientOptions
	signatureHandler, err := embeddednats.NewClientSigHandler([]byte(config.NatsClientOptions.NPrivkey))
	if err != nil {
		return err
	}

	config.EmbeddedNatsOptions = *EmbeddedNatsOptions
	if !config.EmbeddedNatsOptions.NoNatsServer {
		ns := embeddednats.NewEmbeddedServer(signatureHandler.PubKey())
		config.NatsServer = ns
		go ns.Run()
	}

	connHandlers := embeddednats.NewConnHandlers(&config.Log)

	cOpts := nats.Options{
		Url:            config.NatsClientOptions.Url,
		AllowReconnect: false, // TODO: understand how to make this work... setting yes here no connection goes up...
		MaxReconnect:   60,
		// ClosedCB:             nil,
		DisconnectedErrCB: connHandlers.ConnErrHandler,
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
	// NATS out ------------------------------------

	// start the turn generator
	tg := stars.NewTurnGenerator(&config.Log, config.NatsClientConn, config.DB, runner)
	config.TurnGenerator = tg
	go config.TurnGenerator.Run(context.Background())

	return nil
}
