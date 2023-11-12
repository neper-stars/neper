package embeddednats

import (
	"net"

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
	ns *server.Server
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
}

func (en *EmbeddedNats) Shutdown() {
	en.ns.Shutdown()
	en.ns.WaitForShutdown()
}

// InProcessConn is use for testing purposes to get the nats server conn directly
func (en *EmbeddedNats) InProcessConn() (net.Conn, error) {
	return en.ns.InProcessConn()
}
