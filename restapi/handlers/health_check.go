package handlers

import (
	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"

	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/restapi/operations"
)

// HealthCheckDeps holds dependencies for the health check handler
type HealthCheckDeps struct {
	DB          *sqlx.DB
	StarsRunner *stars.Runner
	InitDone    <-chan struct{}
}

// NewHealthCheckHandler creates a health check handler with dependencies
func NewHealthCheckHandler(deps HealthCheckDeps) operations.HealthCheckHandlerFunc {
	return func(params operations.HealthCheckParams) middleware.Responder {
		// Ping database to verify connection is healthy
		if err := deps.DB.PingContext(params.HTTPRequest.Context()); err != nil {
			return operations.NewHealthCheckServiceUnavailable().WithPayload("database unavailable")
		}

		// Check if background initialization is complete
		select {
		case <-deps.InitDone:
			// Initialization complete, check if runner is ready
			if deps.StarsRunner != nil && !deps.StarsRunner.IsReady() {
				return operations.NewHealthCheckOK().WithPayload("ok (stars runner not ready)")
			}
			return operations.NewHealthCheckOK().WithPayload("ok")
		default:
			// Still initializing
			return operations.NewHealthCheckOK().WithPayload("ok (initializing)")
		}
	}
}
