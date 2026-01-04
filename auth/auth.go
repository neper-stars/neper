package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	sq "github.com/Masterminds/squirrel"
	openapi_errors "github.com/go-openapi/errors"
	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"orus.io/orus-io/go-orusapi/database"

	neper "github.com/neper-stars/neper/lib"
	"github.com/neper-stars/neper/models"
)

var (
	// ErrInvalidCredentials is returned when auth fails because the credentials are
	// invalid
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// GenerateAPIKey generates a random API key (32 bytes = 64 hex characters)
func GenerateAPIKey() (string, error) {
	apiKey := make([]byte, 32)
	if _, err := rand.Read(apiKey); err != nil {
		return "", err
	}
	return hex.EncodeToString(apiKey), nil
}

// TokenOptions holds options related to the JWT
type TokenOptions struct {
	Secret             string             `long:"token-secret" env:"TOKEN_SECRET" ini-name:"token-secret" description:"hex HMAC256 secret for signing/verifying JWT (<= 32 bytes)"`
	ExpirationOpt      func(string) error `long:"token-expiration" env:"TOKEN_EXPIRATION" ini-name:"token-expiration" required:"false" description:"Expiration time of the generated tokens" default:"5m"`
	Expiration         time.Duration      `no-flag:"t"`
	CacheMaxSize       int                `long:"token-cache-max-size" env:"TOKEN_CACHE_MAX_SIZE" ini-name:"token-cache-max-size" required:"false" description:"Maximum number of entries in the token cache" default:"10000"`
	CachePurgeDelay    time.Duration      `no-flag:"t"`
	CachePurgeDelayOpt func(string) error `long:"token-cache-purge-delay" env:"TOKEN_CACHE_PURGE_DELAY" ini-name:"token-cache-purge-delay" required:"false" description:"Delay between token cache purges" default:"10m"`
}

func durationOption(tgt *time.Duration) func(string) error {
	return func(duration string) error {
		d, err := time.ParseDuration(duration)
		if err != nil {
			return err
		}
		*tgt = d
		return nil
	}
}

// NewTokenOptions creates a TokenOptions
func NewTokenOptions() *TokenOptions {
	options := TokenOptions{}
	options.ExpirationOpt = durationOption(&options.Expiration)
	options.CachePurgeDelayOpt = durationOption(&options.CachePurgeDelay)
	return &options
}

// Auth handles token authentication & creation
type Auth struct {
	db           *sqlx.DB
	secret       []byte
	cacheMaxSize int

	now func() time.Time

	parser jwt.Parser

	log zerolog.Logger

	cache              *cache.Cache
	collectors         []prometheus.Collector
	cacheHitCollector  prometheus.Counter
	cacheMissCollector prometheus.Counter
}

var authSingleton *Auth

// NewAuth creates an Auth instance
func NewAuth(options TokenOptions, db *sqlx.DB, now func() time.Time, log zerolog.Logger) *Auth {
	// this singleton avoid multi register panics on prometheus during tests
	if authSingleton != nil {
		authSingleton.db = db
		authSingleton.now = now
		authSingleton.log = log
		return authSingleton
	}
	var secret []byte
	if options.Secret == "" {
		log.Warn().Msg("no secret-token provided, will use a random one. This means the auth tokens will be obsoleted if the server restarts")
		secret = make([]byte, 32)
		_, err := rand.Read(secret)
		if err != nil {
			panic(err)
		}
	} else {
		var err error
		secret, err = hex.DecodeString(options.Secret)
		if err != nil {
			panic(err)
		}
	}
	auth := Auth{
		db,
		secret,
		options.CacheMaxSize,
		now,
		jwt.Parser{ValidMethods: []string{"HS256"}},
		log,
		cache.New(options.Expiration, options.CachePurgeDelay),
		nil,
		prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "neper_api_auth_token_cache_hit",
				Help: "Token cache hits",
			},
		),
		prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "neper_api_auth_token_cache_miss",
				Help: "Token cache misses",
			},
		),
	}

	auth.collectors = []prometheus.Collector{
		prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Name: "neper_api_auth_token_cache_size",
				Help: "The number of entries in the token cache",
			}, func() float64 {
				return float64(auth.cache.ItemCount())
			},
		),
		prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Name: "neper_api_auth_token_cache_size_max",
				Help: "The maximum number of entries in the token cache",
			}, func() float64 {
				return float64(auth.cacheMaxSize)
			},
		),
		auth.cacheHitCollector,
		auth.cacheMissCollector,
	}

	for _, c := range auth.collectors {
		prometheus.MustRegister(c)
	}

	authSingleton = &auth
	return &auth
}

// Close release the underlying resources
func (auth *Auth) Close() {
	for _, c := range auth.collectors {
		prometheus.Unregister(c)
	}
}

func (auth *Auth) addToCache(token string, principal *models.Principal) {
	if auth.cache.ItemCount() == auth.cacheMaxSize {
		return
	}
	auth.cache.Set(token, principal, time.Until(time.Unix(principal.ExpiresAt, 0)))
	if auth.cache.ItemCount() == auth.cacheMaxSize {
		// The cache just got full, sending some information if really full
		auth.cache.DeleteExpired()
		if auth.cache.ItemCount() == auth.cacheMaxSize {
			userCounters := map[string]int{}
			for _, v := range auth.cache.Items() {
				p := v.Object.(*models.Principal)
				u := userCounters[p.Subject]
				userCounters[p.Subject] = u + 1
			}
			userCountersNew := map[string]int{}
			for id, count := range userCounters {
				if count > auth.cacheMaxSize/100 {
					userCountersNew[id] = count
				}
			}
			auth.log.Warn().Dict("counters", zerolog.Dict().
				Int("total", auth.cache.ItemCount()).
				Interface("users", userCountersNew),
			).Msg("auth token cache is full")
		}
		return
	}
}

type authDBResult struct {
	models.UserProfileDB
}

func (auth *Auth) BasicAuth(
	ctx context.Context, username, password string,
) (*models.Principal, error) {
	// TODO: note the username is never used ATM
	log := zerolog.Ctx(ctx)
	log.Debug().Msg("attempt authenticate")
	pr, err := auth.Authenticate(ctx, &models.Credentials{
		Nickname: username,
		Apikey:   password,
	})
	if errors.Is(err, ErrInvalidCredentials) {
		return pr, err
	}
	return pr, err
}

// Authenticate authenticates a user
func (auth *Auth) Authenticate(
	ctx context.Context, credentials *models.Credentials,
) (*models.Principal, error) {
	log := zerolog.Ctx(ctx)
	if credentials == nil || credentials.Apikey == "" || credentials.Nickname == "" || credentials.Nickname == neper.SystemUserNickName {
		return nil, ErrInvalidCredentials
	}
	query := database.SQ.
		Select().
		Columns(
			models.UserProfileDBIDColumn,
			models.UserProfileDBNicknameColumn,
			models.UserProfileDBIsManagerColumn,
			models.UserProfileDBPendingColumn,
		).
		From(models.UserProfileDBTable).
		Where(
			sq.And{
				sq.Eq{models.UserProfileDBIsActiveColumn: true},
				sq.Eq{models.UserProfileDBAPIKeyColumn: credentials.Apikey},
				sq.Eq{models.UserProfileDBNicknameColumn: credentials.Nickname},
			},
		)

	var authRes authDBResult
	if err := database.GetContext(ctx, auth.db, &authRes, query, log); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Debug().Msg("auth: no matching profile")
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	return &models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   authRes.ID,
			ExpiresAt: 0,
		},
		IsGlobalManager: authRes.IsManager,
		IsPending:       authRes.Pending,
		NickName:        authRes.Nickname,
	}, nil
}

var setJWTNowMutex sync.Mutex

func setJWTNow(now func() time.Time) func() {
	setJWTNowMutex.Lock()
	s := jwt.TimeFunc
	jwt.TimeFunc = now
	return func() {
		jwt.TimeFunc = s
		setJWTNowMutex.Unlock()
	}
}

// Auth loads an auth header and returns a Principal instance
func (auth *Auth) Auth(header string) (*models.Principal, error) {
	if !strings.HasPrefix(header, "Bearer ") {
		return nil, openapi_errors.New(
			http.StatusUnauthorized,
			"Invalid Authorization header value. Expects 'Bearer <token>', got '%s'",
			header)
	}
	tokenString := header[7:]

	if value, ok := auth.cache.Get(tokenString); ok {
		auth.cacheHitCollector.Inc()
		return value.(*models.Principal), nil
	}
	auth.cacheMissCollector.Inc()

	defer setJWTNow(auth.now)()
	token, err := auth.parser.ParseWithClaims(
		tokenString,
		&models.Principal{},
		func(token *jwt.Token) (interface{}, error) {
			return auth.secret, nil
		})
	if err != nil {
		return nil, openapi_errors.New(http.StatusUnauthorized, "Invalid token: %s", err)
	}
	if !token.Valid {
		return nil, openapi_errors.New(http.StatusUnauthorized, "Invalid token")
	}
	claims := token.Claims.(*models.Principal)
	auth.addToCache(tokenString, claims)
	return claims, nil
}

// MakeToken creates a new token
func (auth *Auth) MakeToken(principal models.Principal) (string, error) {
	principal.ExpiresAt = auth.now().Add(time.Minute * 5).Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, principal)
	sToken, err := token.SignedString(auth.secret)
	if err != nil {
		return "", err
	}
	auth.addToCache(sToken, &principal)
	return sToken, nil
}
