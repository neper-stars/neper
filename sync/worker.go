package sync

import (
	"context"
	"fmt"
	"io"

	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

// Worker receive all messages and dispatch them in the database
type Worker struct {
	db     *sqlx.DB
	logger zerolog.Logger
}

// NewWorker creates a Worker
func NewWorker(db *sqlx.DB, log zerolog.Logger) (*Worker, error) {
	return &Worker{db, log}, nil
}

// DispatchMessage dispatches a single incoming message
func (w *Worker) DispatchMessage(ctx context.Context, msgType string, data io.Reader, log zerolog.Logger) error {
	timer := prometheus.NewTimer(consumerMessageDispatchDuration.WithLabelValues(msgType))
	defer timer.ObserveDuration()

	switch msgType {
	case NeperReferentialBroadcast:
		return w.handleNeperReferentialBroadcast(ctx, data, log)
	default:
		return fmt.Errorf("unknown message type: %s", msgType)
	}
}

var (
	consumerMessageDispatchDuration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "sync_consumer_dispatch_duration",
			Help:       "sync input message dispatch/handling duration",
			Objectives: map[float64]float64{0.9: 0.01, 0.95: 0.01, 0.99: 0.001},
		},
		[]string{"msgtype"},
	)
)
