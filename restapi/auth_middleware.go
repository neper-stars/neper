package restapi

import (
	"context"
	"encoding/base64"
	"net/http"
	"regexp"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/neper-stars/neper/auth"
	"github.com/neper-stars/neper/models"
)

type AuthMiddlewareConfig struct {
	Auth *auth.Auth
	Log  zerolog.Logger
}

var (
	publicAccess = []*regexp.Regexp{
		// frontend app
		regexp.MustCompile("^/$"),
		regexp.MustCompile("^/main.js$"),
		regexp.MustCompile("^/reload/reload.js$"),
		regexp.MustCompile("^/dist/"),
		regexp.MustCompile("^/sessions"),
		regexp.MustCompile("^/sign-in"),
		regexp.MustCompile("^/sign-out"),
		// statics
		regexp.MustCompile("^/assets"),
		regexp.MustCompile("^/favicon.ico"),
		// auth
		regexp.MustCompile("^/api/v1/auth/authenticate$"),
		regexp.MustCompile("^/api/v1/auth/register$"),
		regexp.MustCompile("^/assets"),
		regexp.MustCompile("^/docs$"),
		regexp.MustCompile("^/swagger.json$"),
		// metrics
		regexp.MustCompile("^/metrics$"),
	}
	basicAuthAccess = []*regexp.Regexp{
		regexp.MustCompile("^/swagger.json$"),
		regexp.MustCompile("^/api/docs$"),
		regexp.MustCompile("^/api/docs/"),
	}
)

type ContextIdentityType int

const ContextIdentity ContextIdentityType = iota

func GetPrincipal(r *http.Request) *models.Principal {
	if v := r.Context().Value(ContextIdentity); v != nil {
		return v.(*models.Principal)
	}
	return nil
}

func matchPublicAccess(path string) bool {
	for _, r := range publicAccess {
		if r.MatchString(path) {
			return true
		}
	}
	return false
}

func matchBasicAuthAccess(path string) bool {
	for _, r := range basicAuthAccess {
		if r.MatchString(path) {
			return true
		}
	}
	return false
}

// AuthMiddleware authenticates the user
func AuthMiddleware(config AuthMiddlewareConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")

			if matchBasicAuthAccess(r.URL.Path) {
				if strings.HasPrefix(authHeader, "Basic ") {
					data, err := base64.StdEncoding.DecodeString(authHeader[6:])
					if err != nil {
						log.Err(err).Msg("error decoding basic auth header")
						rw.Header().Set("WWW-Authenticate", "Basic realm=\"Neper API\"")
						rw.WriteHeader(http.StatusUnauthorized)
						return
					}
					if i := strings.Index(string(data), ":"); i != -1 {
						pr, err := config.Auth.BasicAuth(r.Context(), string(data[:i]), string(data[i+1:]))
						if err != nil {
							log.Err(err).Msg("authentication error")
							rw.Header().Set("WWW-Authenticate", "Basic realm=\"Neper API\"")
							rw.WriteHeader(http.StatusUnauthorized)
							return
						}
						// We're all good, the user is authenticated
						logger := zerolog.Ctx(r.Context())
						logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
							return c.Object("user", pr)
						})
						r = r.WithContext(context.WithValue(r.Context(), ContextIdentity, pr))
						next.ServeHTTP(rw, r)
						return
					}
				} else {
					next.ServeHTTP(rw, r)
					return
				}
			}
			if authHeader != "" {
				pr, err := config.Auth.Auth(authHeader)
				if err != nil {
					logger := zerolog.Ctx(r.Context())
					logger.Err(err).Msg("error reading authorization header")
				} else {
					logger := zerolog.Ctx(r.Context())
					logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
						return c.Object("user", pr)
					})
					r = r.WithContext(context.WithValue(r.Context(), ContextIdentity, pr))
					next.ServeHTTP(rw, r)
					return
				}
			} else if matchPublicAccess(r.URL.Path) {
				next.ServeHTTP(rw, r)
				return
			}
			rw.WriteHeader(http.StatusUnauthorized)
		})
	}
}
