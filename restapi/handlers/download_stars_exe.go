package handlers

import (
	"bytes"
	"io"
	"net/http"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/rs/zerolog"

	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewDownloadStarsExeHandler creates a handler for downloading the Stars! executable
func NewDownloadStarsExeHandler(log *zerolog.Logger) *DownloadStarsExeHandler {
	return &DownloadStarsExeHandler{log: log}
}

// DownloadStarsExeHandler handles the /v1/downloads/stars.exe endpoint
type DownloadStarsExeHandler struct {
	log *zerolog.Logger
}

// Handle handles the request
func (h *DownloadStarsExeHandler) Handle(
	params operations.DownloadStarsExeParams, principal *models.Principal,
) middleware.Responder {
	exeBytes := stars.GetStarsExecutable()
	reader := io.NopCloser(bytes.NewReader(exeBytes))

	return &downloadStarsExeResponse{
		payload: reader,
		log:     h.log,
	}
}

// downloadStarsExeResponse is a custom responder that sets proper headers for file download
type downloadStarsExeResponse struct {
	payload io.ReadCloser
	log     *zerolog.Logger
}

func (r *downloadStarsExeResponse) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {
	rw.Header().Set("Content-Type", "application/octet-stream")
	rw.Header().Set("Content-Disposition", `attachment; filename="stars.exe"`)
	rw.WriteHeader(200)
	if r.payload != nil {
		defer func() {
			if err := r.payload.Close(); err != nil {
				r.log.Err(err).Msg("failed to close payload reader")
			}
		}()
		_, _ = io.Copy(rw, r.payload)
	}
}
