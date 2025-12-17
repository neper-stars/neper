package embeddednats

type ClientOptions struct {
	Url      string `long:"nats-url" env:"NATS_SERVER_URL" ini-name:"nats-url" description:"nats server url for service communications." default:"nats://localhost:4222"`
	NPrivkey string `long:"nkey" env:"NATS_CLIENT_NKEY" ini-name:"nkey" description:"private part of the nKey to connect to the nats server. Begins with an S"`
}

func NewNatsClientOptions() *ClientOptions {
	options := ClientOptions{}
	return &options
}
