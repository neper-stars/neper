// This file is generated only once and is safe to edit

package cmd

import (
	"github.com/go-openapi/swag"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/rs/zerolog"

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
	{
		runner, err := stars.NewRunner(&config.Log, StarsRunnerOptions)
		if err != nil {
			return err
		}
		config.StarsRunner = runner
	}
	if err := config.StarsRunner.Init(); err != nil {
		return err
	}

	// ----------------------------
	// NATS configuration
	// ----------------------------
	config.NatsClientOptions = NatsClientOptions
	signatureHandler, err := NewClientSigHandler([]byte(config.NatsClientOptions.NPrivkey))
	if err != nil {
		return err
	}

	config.EmbeddedNatsOptions = *EmbeddedNatsOptions
	if !config.EmbeddedNatsOptions.NoNatsServer {
		ns := embeddednats.NewEmbeddedServer(signatureHandler.PubKey())
		config.NatsServer = ns
		go ns.Run()
	}

	connHandlers := NewConnHandlers(&config.Log)

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

	return nil
}

type ConnHandlers struct {
	logger *zerolog.Logger
}

func NewConnHandlers(zl *zerolog.Logger) *ConnHandlers {
	return &ConnHandlers{
		logger: zl,
	}
}

func (ch *ConnHandlers) ConnHandler(conn *nats.Conn) {
	ch.logger.Info().
		Str("connStatus", conn.Status().String()).
		Msg("nats client connected successfully")
}

func (ch *ConnHandlers) ConnErrHandler(conn *nats.Conn, err error) {
	ch.logger.Err(err).
		Str("connStatus", conn.Status().String()).
		Msg("nats client failed to connect")
}

func (ch *ConnHandlers) ErrHandler(conn *nats.Conn, sub *nats.Subscription, err error) {
	ch.logger.Err(err).
		Str("connStatus", conn.Status().String()).
		Str("subject", sub.Subject).
		Msg("nats client error")
}

// ClientSigHandler is used to sign the nonce that the server will send us as
// a challenge.
// We need the keypair.
// As a helper we also provide a way to get the PublicKey back
type ClientSigHandler struct {
	kp     nkeys.KeyPair
	pubKey string
}

// PubKey returns the public key part derived from the private key
func (sh *ClientSigHandler) PubKey() string {
	return sh.pubKey
}

// NewClientSigHandler constructs a new ClientSigHandler
// This is the recommended way to create one.
// This can return an error if the provided nkey cannot be
// parsed to obtain a nkeys.KeyPair
func NewClientSigHandler(nkey []byte) (*ClientSigHandler, error) {
	kp, err := nkeys.ParseDecoratedNKey(nkey)
	if err != nil {
		return nil, err
	}
	pub, err := kp.PublicKey()
	if err != nil {
		return nil, err
	}
	return &ClientSigHandler{
		kp:     kp,
		pubKey: pub,
	}, nil
}

// Sign it the callback function that will be passed to the NATS.io client.
func (sh *ClientSigHandler) Sign(nonce []byte) ([]byte, error) {
	return sh.kp.Sign(nonce)
}
