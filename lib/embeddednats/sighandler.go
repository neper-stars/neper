package embeddednats

import "github.com/nats-io/nkeys"

// ClientSigHandler is used to sign the nonce that the server will send us as
// a challenge.
// We need the keypair.
// As a helper we also provide a way to get the PublicKey back
type ClientSigHandler struct {
	kp     nkeys.KeyPair
	pubKey string
}

// PubKey returns the public key part derived from the private key
func (sh *ClientSigHandler) PubKey() string {
	return sh.pubKey
}

// NewClientSigHandler constructs a new ClientSigHandler
// This is the recommended way to create one.
// This can return an error if the provided nkey cannot be
// parsed to obtain a nkeys.KeyPair
func NewClientSigHandler(nkey []byte) (*ClientSigHandler, error) {
	kp, err := nkeys.ParseDecoratedNKey(nkey)
	if err != nil {
		return nil, err
	}
	pub, err := kp.PublicKey()
	if err != nil {
		return nil, err
	}
	return &ClientSigHandler{
		kp:     kp,
		pubKey: pub,
	}, nil
}

// Sign it the callback function that will be passed to the NATS.io client.
func (sh *ClientSigHandler) Sign(nonce []byte) ([]byte, error) {
	return sh.kp.Sign(nonce)
}
