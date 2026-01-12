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
	"github.com/rs/zerolog/hlog"
	"orus.io/orus-io/go-orusapi"

	"github.com/neper-stars/neper/auth"
	"github.com/neper-stars/neper/lib/embeddednats"
	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/lib/race"
	"github.com/neper-stars/neper/lib/racefiles"
	"github.com/neper-stars/neper/lib/registration"
	"github.com/neper-stars/neper/lib/session"
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
	NotifyService       *notify.Service
	RaceProcessor       *racefiles.Processor

	// Registration options
	RegistrationOptions *registration.Options
	RegistrationLimiter *registration.RateLimiter

	// Race options
	RaceOptions *race.Options

	// Session options
	SessionOptions *session.Options

	// InitDone is closed when background initialization is complete
	// (serial keys loaded, wine prefix initialized)
	InitDone chan struct{}
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
	// Use the pre-initialized authenticator from config
	api.KeyAuth = config.Authenticator.Auth
	config.OnShutdown(config.Authenticator.Close)
	config.OnShutdown(config.StarsRunner.Shutdown)
	config.OnShutdown(config.TurnGenerator.Shutdown)
	// Close NATS client connection before shutting down the server
	// This ensures clean disconnection and prevents "CLOSED" state errors
	// in handlers that use the connection during shutdown
	if config.NatsClientConn != nil {
		config.OnShutdown(func() {
			config.NatsClientConn.Close()
		})
	}
	if config.NatsServer != nil {
		config.OnShutdown(config.NatsServer.Shutdown)
	}

	// Health check (no auth, pings database and reports initialization status)
	api.HealthCheckHandler = handlers.NewHealthCheckHandler(handlers.HealthCheckDeps{
		DB:          config.DB,
		StarsRunner: config.StarsRunner,
		InitDone:    config.InitDone,
	})

	// Authenticate
	api.AuthenticateHandler = handlers.NewAuthenticateHandler(config.DB, config.Authenticator)
	api.RefreshTokenHandler = handlers.NewRefreshTokenHandler(&config.Log, config.DB, config.Authenticator)

	// Registration (public endpoint - no auth required)
	api.RegisterHandler = handlers.NewRegisterHandler(&config.Log, config.DB, config.RegistrationOptions, config.RegistrationLimiter, config.NotifyService)

	// Pending registrations management (global managers only)
	api.PendingRegistrationListHandler = handlers.NewPendingRegistrationListHandler(&config.Log, config.DB)
	api.PendingRegistrationApproveHandler = handlers.NewPendingRegistrationApproveHandler(&config.Log, config.DB, config.NotifyService)
	api.PendingRegistrationRejectHandler = handlers.NewPendingRegistrationRejectHandler(&config.Log, config.DB, config.NotifyService)

	// Sessions
	api.SessionsListHandler = handlers.NewSessionsListHandler(config.DB)
	api.SessionReadHandler = handlers.NewSessionReadHandler(&config.Log, config.DB)
	api.SessionCreateHandler = handlers.NewSessionCreateHandler(config.DB, config.NotifyService, config.SessionOptions)
	api.SessionUpdateHandler = handlers.NewSessionUpdateHandler(&config.Log, config.DB)
	api.SessionDeleteHandler = handlers.NewSessionDeleteHandler(&config.Log, config.DB, config.NotifyService)
	api.SessionJoinHandler = handlers.NewSessionJoinHandler(&config.Log, config.DB, config.NotifyService, config.SessionOptions)
	api.SessionQuitHandler = handlers.NewSessionQuitHandler(&config.Log, config.DB, config.NotifyService)
	api.SessionPromoteMemberHandler = handlers.NewSessionPromoteMemberHandler(&config.Log, config.DB, config.NotifyService)
	api.SessionArchiveHandler = handlers.NewSessionArchiveHandler(&config.Log, config.DB, config.NotifyService)

	// UserProfiles
	api.UserProfileCreateHandler = handlers.NewUserProfileCreateHandler(config.DB)
	api.UserProfileReadHandler = handlers.NewUserProfileReadHandler(config.DB)
	api.UserProfileListHandler = handlers.NewUserProfilesListHandler(config.DB)
	api.UserProfileUpdateHandler = handlers.NewUserProfileUpdateHandler(config.DB)
	api.UserProfileDeleteHandler = handlers.NewUserProfileDeleteHandler(&config.Log, config.DB)
	api.UserProfileResetApikeyHandler = handlers.NewUserProfileResetApikeyHandler(config.DB)

	// Races
	api.RaceCreateHandler = handlers.NewRaceCreateHandler(&config.Log, config.DB, config.RaceProcessor, config.NotifyService, config.RaceOptions)
	api.RacesListHandler = handlers.NewRacesListHandler(config.DB)
	api.RaceReadHandler = handlers.NewRaceReadHandler(config.DB)
	api.RaceDeleteHandler = handlers.NewRaceDeleteHandler(config.DB, config.NotifyService)

	// Invitations
	api.InvitationCreateHandler = handlers.NewInvitationCreateHandler(&config.Log, config.DB, config.NotifyService)
	api.InvitationListHandler = handlers.NewInvitationListHandler(&config.Log, config.DB)
	api.InvitationSentListHandler = handlers.NewInvitationSentListHandler(&config.Log, config.DB)
	api.InvitationAcceptHandler = handlers.NewInvitationAcceptHandler(&config.Log, config.DB, config.NotifyService)
	api.InvitationDeclineHandler = handlers.NewInvitationDeclineHandler(&config.Log, config.DB, config.NotifyService)

	// Session Player Race mapping (once player is in a session, he sets his race for this session)
	api.SessionPlayerRaceCreateHandler = handlers.NewSessionPlayerRaceCreateHandler(&config.Log, config.DB, config.NotifyService)
	api.SessionPlayerRaceGetHandler = handlers.NewSessionPlayerRaceGetHandler(config.DB)
	api.SessionPlayerRaceSetReadyHandler = handlers.NewSessionPlayerRaceSetReadyHandler(&config.Log, config.DB, config.NotifyService)
	api.SessionPlayerRaceDeleteHandler = handlers.NewSessionPlayerRaceDeleteHandler(config.DB, config.NotifyService)
	// player order on a session
	api.ReorderPlayersHandler = handlers.NewReorderPlayersHandler(&config.Log, config.DB, config.NotifyService)
	// ruleset
	api.RulesCreateHandler = handlers.NewRulesCreateHandler(&config.Log, config.DB, config.NotifyService)
	api.RulesReadHandler = handlers.NewRulesReadHandler(&config.Log, config.DB)

	// game creation
	api.GameCreateHandler = handlers.NewGameCreateHandler(&config.Log, config.DB, config.StarsRunner, config.NotifyService)
	// turn get (each player its own call to get its own files)
	api.TurnGetHandler = handlers.NewTurnGetHandler(&config.Log, config.DB)
	// turn latest (get the most recent turn for a session)
	api.TurnLatestHandler = handlers.NewTurnLatestHandler(&config.Log, config.DB)
	// session files get (managers only - returns all files including host file)
	api.SessionFilesGetHandler = handlers.NewSessionFilesGetHandler(&config.Log, config.DB)

	turnSubmitter := sessionSubmitter.NewSessionSubmitter(&config.Log, config.NatsClientConn)
	// switch player to AI control (managers only - modifies .hst file)
	api.SessionPlayerSwitchToAIHandler = handlers.NewSessionPlayerSwitchToAIHandler(&config.Log, config.DB, turnSubmitter, config.NotifyService)
	// switch player to human control (managers only - modifies .hst file)
	api.SessionPlayerSwitchToHumanHandler = handlers.NewSessionPlayerSwitchToHumanHandler(&config.Log, config.DB)
	// get player control status (managers only)
	api.SessionPlayerControlHandler = handlers.NewSessionPlayerControlHandler(&config.Log, config.DB)

	api.TurnSubmitHandler = handlers.NewTurnSubmitHandler(&config.Log, config.DB, turnSubmitter, config.NotifyService)
	// orders status (list of players who have submitted orders)
	api.OrdersStatusHandler = handlers.NewOrdersStatusHandler(&config.Log, config.DB)

	// for user to be able to find its own ID
	api.UserinfoHandler = handlers.NewUserinfoHandler(config.DB)

	// Notifications (WebSocket)
	api.NotificationsHandler = handlers.NewNotificationsHandler(&config.Log, config.DB, config.NatsClientConn)

	// Downloads
	api.DownloadStarsExeHandler = handlers.NewDownloadStarsExeHandler(&config.Log)

	// Historic Backup
	api.HistoricBackupHandler = handlers.NewHistoricBackupHandler(&config.Log, config.DB)

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

// pathAwareAccessHandler creates an access handler that logs health check requests at DEBUG level
// and all other requests at INFO level (for 2xx), or WARN/ERROR for 4xx/5xx.
func pathAwareAccessHandler(status4XXLogLevel zerolog.Level) func(r *http.Request, status, size int, duration time.Duration) {
	return func(r *http.Request, status, size int, duration time.Duration) {
		log := hlog.FromRequest(r)

		var event *zerolog.Event
		//nolint:zerologlint
		switch {
		case status >= http.StatusInternalServerError:
			event = log.Error()
		case status >= http.StatusBadRequest:
			event = log.WithLevel(status4XXLogLevel)
		default:
			// Log health check at DEBUG level, everything else at INFO
			if r.URL.Path == "/api/healthz" {
				event = log.Debug()
			} else {
				event = log.Info()
			}
		}

		event.
			Dict("request", zerolog.Dict().
				Str("method", r.Method).
				Str("url", r.URL.String())).
			Int("status", status).
			Int("size", size).
			Dur("duration", duration).
			Msg(http.StatusText(status))
	}
}

// The middleware configuration happens before anything, this middleware also applies to serving the swagger.json document.
// So this is a good place to plug in a panic handling middleware, logging and metrics
func setupGlobalMiddleware(config Config) func(handler http.Handler) http.Handler {
	// Custom log stack that logs health checks at DEBUG level
	logStack := []alice.Constructor{
		hlog.NewHandler(config.Log),
		hlog.AccessHandler(pathAwareAccessHandler(zerolog.WarnLevel)),
		hlog.RemoteAddrHandler("ip"),
		hlog.UserAgentHandler("user_agent"),
		hlog.RefererHandler("referer"),
		hlog.RequestIDHandler("req_id", "Request-Id"),
		orusapi.CatchPanics,
	}

	stack := alice.New(logStack...)
	stack = stack.Append(
		AuthMiddleware(AuthMiddlewareConfig{
			Auth: config.Authenticator,
			Log:  config.Log}),
	)

	return stack.Then
}
