// This file is safe to edit. Once it exists it will not be overwritten

package restapi

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	"github.com/jmoiron/sqlx"
	"github.com/justinas/alice"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi"

	"github.com/neper-stars/neper/auth"
	"github.com/neper-stars/neper/lib/embeddednats"
	"github.com/neper-stars/neper/lib/sessionSubmitter"
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/restapi/handlers"
	"github.com/neper-stars/neper/restapi/operations"
)

// Callback is a simple function
type Callback func()

// Config holds the things required to configure the API
type Config struct {
	Log zerolog.Logger
	DB  *sqlx.DB

	BaseURL  string
	Shutdown []Callback
	// Here you can add anything the ConfigureAPI function will need to set up
	// the API

	Now                 func() time.Time
	TokenOptions        auth.TokenOptions
	Authenticator       *auth.Auth // Authentication
	StarsRunner         *stars.Runner
	TurnGenerator       *stars.TurnGenerator
	EmbeddedNatsOptions embeddednats.ServerOptions
	NatsServer          *embeddednats.EmbeddedNats
	NatsClientOptions   *embeddednats.ClientOptions
	NatsClientConn      *nats.Conn
}

// OnShutdown add a shutdown callback
func (c *Config) OnShutdown(cb Callback) {
	c.Shutdown = append(c.Shutdown, cb)
}

//go:generate swagger generate server --target ../../neper --name Neper --spec ../neper-api.yaml --template-dir dependencies/go-orusapi/templates --principal models.Principal

type ConfiguredAPI struct {
	api     *operations.NeperAPI
	handler http.Handler

	config Config
}

func (capi *ConfiguredAPI) Name() string {
	return "Neper"
}

func (capi *ConfiguredAPI) Handler() http.Handler {
	return capi.handler
}

func (capi *ConfiguredAPI) PreServerShutdown() {
}

func (capi *ConfiguredAPI) ServerShutdown() {
	lSize := len(capi.config.Shutdown)
	for i := range capi.config.Shutdown {
		capi.config.Shutdown[lSize-i-1]()
	}
}

func ConfigureAPI(api *operations.NeperAPI, server *orusapi.Server, config Config) (*ConfiguredAPI, error) {
	// configure the api here
	api.ServeError = errors.ServeError

	if config.BaseURL == "" {
		scheme := "https"
		if len(server.EnabledListeners) != 0 {
			scheme = server.EnabledListeners[0]
		}
		config.BaseURL = fmt.Sprintf("%s://%s:%d",
			scheme, server.Host, server.Port,
		)
	}

	// Set your custom logger if needed. Default one is log.Printf
	// Expected interface func(string, ...interface{})
	//
	// Example:
	// api.Logger = log.Printf

	api.UseSwaggerUI()
	// To continue using redoc as your UI, uncomment the following line
	// api.UseRedoc()

	api.JSONConsumer = runtime.JSONConsumer()

	api.JSONProducer = runtime.JSONProducer()

	// Applies when the "Authorization" header is set
	apiAuth := auth.NewAuth(config.TokenOptions, config.DB, config.Now, config.Log)
	api.KeyAuth = apiAuth.Auth
	config.OnShutdown(apiAuth.Close)
	config.OnShutdown(config.StarsRunner.Shutdown)
	config.OnShutdown(config.TurnGenerator.Shutdown)
	config.OnShutdown(config.NatsServer.Shutdown)

	// Authenticate
	api.AuthenticateHandler = handlers.NewAuthenticateHandler(config.DB, apiAuth)
	api.RefreshTokenHandler = handlers.NewRefreshTokenHandler(&config.Log, config.DB, config.Authenticator)

	// Sessions
	api.SessionsListHandler = handlers.NewSessionsListHandler(config.DB)
	api.SessionReadHandler = handlers.NewSessionReadHandler(&config.Log, config.DB)
	api.SessionCreateHandler = handlers.NewSessionCreateHandler(config.DB)
	api.SessionUpdateHandler = handlers.NewSessionUpdateHandler(&config.Log, config.DB)

	// UserProfiles
	api.UserProfileCreateHandler = handlers.NewUserProfileCreateHandler(config.DB)
	api.UserProfileReadHandler = handlers.NewUserProfileReadHandler(config.DB)
	api.UserProfileListHandler = handlers.NewUserProfilesListHandler(config.DB)
	api.UserProfileUpdateHandler = handlers.NewUserProfileUpdateHandler(config.DB)

	// Races
	api.RaceCreateHandler = handlers.NewRaceCreateHandler(&config.Log, config.DB)
	api.RacesListHandler = handlers.NewRacesListHandler(config.DB)
	api.RaceReadHandler = handlers.NewRaceReadHandler(config.DB)

	// Invitations
	api.InvitationCreateHandler = handlers.NewInvitationCreateHandler(&config.Log, config.DB)
	api.InvitationListHandler = handlers.NewInvitationListHandler(&config.Log, config.DB)
	api.InvitationAcceptHandler = handlers.NewInvitationAcceptHandler(&config.Log, config.DB)

	// Session Player Race mapping (once player is in a session, he sets his race for this session)
	api.SessionPlayerRaceCreateHandler = handlers.NewSessionPlayerRaceCreateHandler(&config.Log, config.DB)
	// player order on a session
	api.ReorderPlayersHandler = handlers.NewReorderPlayersHandler(&config.Log, config.DB)
	// ruleset
	api.RulesCreateHandler = handlers.NewRulesCreateHandler(&config.Log, config.DB)

	// game creation
	api.GameCreateHandler = handlers.NewGameCreateHandler(&config.Log, config.DB, config.StarsRunner)
	// turn get (each player its own call to get its own files)
	api.TurnGetHandler = handlers.NewTurnGetHandler(&config.Log, config.DB, config.NatsClientConn)

	turnSubmitter := sessionSubmitter.NewSessionSubmitter(&config.Log, config.NatsClientConn)
	api.TurnSubmitHandler = handlers.NewTurnSubmitHandler(&config.Log, config.DB, turnSubmitter)

	// for user to be able to find its own ID
	api.UserinfoHandler = handlers.NewUserinfoHandler(config.DB)

	if server.Prometheus {
		api.PrometheusInstrumentHandlers()
	}
	api.LoggingInstrumentHandlers()

	return &ConfiguredAPI{
		api:     api,
		handler: setupGlobalMiddleware(config)(api.Serve(setupMiddlewares)),
		config:  config,
	}, nil
}

// The TLS configuration before HTTPS server starts.
// Make all necessary changes to the TLS configuration here.
// func configureTLS(tlsConfig *tls.Config) {
// }

// As soon as server is initialized but not run yet, this function will be called.
// If you need to modify a config, store server instance to stop it individually later, this is the place.
// This function can be called multiple times, depending on the number of serving schemes.
// scheme value will be set accordingly: "http", "https" or "unix"
// func configureServer(s *http.Server, scheme, addr string) {
// }

// The middleware configuration is for the handler executors. These do not apply to the swagger.json document.
// The middleware executes after routing but before authentication, binding and validation
func setupMiddlewares(handler http.Handler) http.Handler {
	return handler
}

// The middleware configuration happens before anything, this middleware also applies to serving the swagger.json document.
// So this is a good place to plug in a panic handling middleware, logging and metrics
func setupGlobalMiddleware(config Config) func(handler http.Handler) http.Handler {
	stack := alice.New(
		orusapi.LogStack(config.Log, zerolog.WarnLevel)...,
	)
	stack = stack.Append(
		AuthMiddleware(AuthMiddlewareConfig{
			Auth: config.Authenticator,
			Log:  config.Log}),
	)

	return stack.Then
}
