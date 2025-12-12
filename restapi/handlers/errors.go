package handlers

import (
	"net/http"

	"github.com/go-openapi/runtime"
	"github.com/rs/zerolog"

	"github.com/neper-stars/neper/models"
)

// NotFound is a helper function to log and reply a not found error
func NotFound(msg string, log *zerolog.Logger) *ErrorResponder {
	log.Warn().Str("message", msg).Msg("not found error")
	return &ErrorResponder{
		code: http.StatusNotFound,
		err: &models.Error{
			Code:    http.StatusNotFound,
			Message: &msg,
		},
	}
}

// BadRequest is a helper function to log and reply a bad request error
func BadRequest(msg string, log *zerolog.Logger) *ErrorResponder {
	log.Warn().Str("message", msg).Msg("bad request error")
	return &ErrorResponder{
		code: http.StatusBadRequest,
		err: &models.Error{
			Code:    http.StatusBadRequest,
			Message: &msg,
		},
	}
}

// BadRequestErr is a helper function to log and reply a bad request error
func BadRequestErr(err error, log *zerolog.Logger) *ErrorResponder {
	log.Warn().Err(err).Msg("bad request error")
	return &ErrorResponder{code: http.StatusBadRequest, err: models.FromError(err)}
}

// Conflict is a helper function to log and reply a conflict error
func Conflict(msg string, log *zerolog.Logger) *ErrorResponder {
	log.Warn().Str("message", msg).Msg("conflict error")
	return &ErrorResponder{
		code: http.StatusConflict,
		err: &models.Error{
			Code:    http.StatusConflict,
			Message: &msg,
		},
	}
}

// PreconditionFailed is a helper function to log and reply a precondition failed error
func PreconditionFailed(msg string, log *zerolog.Logger) *ErrorResponder {
	log.Warn().Str("message", msg).Msg("precondition failed error")
	return &ErrorResponder{
		code: http.StatusPreconditionFailed,
		err: &models.Error{
			Code:    http.StatusPreconditionFailed,
			Message: &msg,
		},
	}
}

type ErrorResponder struct {
	code int
	err  *models.Error
}

// WriteResponse to the client
func (r *ErrorResponder) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {
	rw.WriteHeader(r.code)
	if r.err != nil {
		if err := producer.Produce(rw, models.FromError(r.err)); err != nil {
			panic(err) // let the recovery middleware deal with this
		}
	}
}

// InternalError is a helper function to log and internal error and obtain
// a Responder that you can use as a return value in your handler
func InternalError(err error, log *zerolog.Logger, debug bool) *InternalErrorResponder {
	log.Err(err).Msg("internal error")
	return &InternalErrorResponder{err: err, log: log, debug: debug}
}

// InternalErrorResponder ...
// InternalErrorResponder is a helper to filter out the error content sent
// to the client this filtering is dependent on the global debug flag.
// ie: if debug is set then the client will receive the detail
// of the internal error.
// This is something you obviously do NOT want to do in production mode.
type InternalErrorResponder struct {
	err   error
	log   *zerolog.Logger
	debug bool
}

// WriteResponse to the client
func (r *InternalErrorResponder) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {
	rw.WriteHeader(500)
	if r.err != nil && r.debug {
		// clients are allowed to see internal errors content only if the
		// global debug mode is activated on the api
		if err := producer.Produce(rw, models.FromError(r.err)); err != nil {
			panic(err) // let the recovery middleware deal with this
		}
	}
}
