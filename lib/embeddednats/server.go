package embeddednats

import (
	"net"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

type ServerOptions struct {
	NoNatsServer bool `long:"no-nats-server" description:"disallow automatically running an embedded NATS.io server for inter service communication"`
}

func NewEmbeddedNatsOptions() *ServerOptions {
	options := ServerOptions{}
	return &options
}

type EmbeddedNats struct {
	ns      *server.Server
	started bool
}

func NewEmbeddedServer(nKey string) *EmbeddedNats {
	opts := &server.Options{
		Nkeys: []*server.NkeyUser{
			{
				// go install github.com/nats-io/nkeys/nk@latest &&\
				// nk -gen user -pubout
				Nkey: nKey,
			},
		},
		// Disable NATS server's internal signal handling to prevent double shutdown.
		// Our application handles signals and calls Shutdown() explicitly.
		// See: https://github.com/nats-io/nats-server/issues/5922
		NoSigs: true,
	}
	ns, err := server.NewServer(opts)
	if err != nil {
		panic(err)
	}
	return &EmbeddedNats{ns: ns}
}

// Run is intended to be run in a goroutine
func (en *EmbeddedNats) Run() {
	en.ns.Start()
	en.started = true
}

// WaitReady waits for the NATS server to be ready for connections
func (en *EmbeddedNats) WaitReady(timeout time.Duration) bool {
	return en.ns.ReadyForConnections(timeout)
}

func (en *EmbeddedNats) Shutdown() {
	if !en.started {
		return
	}
	en.ns.Shutdown()
	en.ns.WaitForShutdown()
}

// InProcessConn is use for testing purposes to get the nats server conn directly
func (en *EmbeddedNats) InProcessConn() (net.Conn, error) {
	return en.ns.InProcessConn()
}
