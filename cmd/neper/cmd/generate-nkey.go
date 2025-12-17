package cmd

import (
	"fmt"

	"github.com/nats-io/nkeys"
)

// GenerateNkeyCmd generates NATS nkeys for authentication
type GenerateNkeyCmd struct {
	Type string `long:"type" short:"t" choice:"user" choice:"account" choice:"operator" choice:"server" choice:"cluster" default:"user" description:"type of nkey to generate"`
}

// Execute generates and prints a nkey pair
func (cmd *GenerateNkeyCmd) Execute([]string) error {
	var kp nkeys.KeyPair
	var err error

	switch cmd.Type {
	case "user":
		kp, err = nkeys.CreateUser()
	case "account":
		kp, err = nkeys.CreateAccount()
	case "operator":
		kp, err = nkeys.CreateOperator()
	case "server":
		kp, err = nkeys.CreateServer()
	case "cluster":
		kp, err = nkeys.CreateCluster()
	default:
		return fmt.Errorf("unknown key type: %s", cmd.Type)
	}

	if err != nil {
		return fmt.Errorf("failed to create %s key: %w", cmd.Type, err)
	}

	seed, err := kp.Seed()
	if err != nil {
		return fmt.Errorf("failed to get seed: %w", err)
	}

	pub, err := kp.PublicKey()
	if err != nil {
		return fmt.Errorf("failed to get public key: %w", err)
	}

	fmt.Printf("Seed (private key - keep secret - use this in your config):\n%s\n\n", string(seed))
	fmt.Printf("Public key (needed when using an *external* Nats server only):\n%s\n", pub)

	return nil
}

func init() {
	cmd := &GenerateNkeyCmd{}

	_, err := parser.AddCommand("generate-nkey", "Generate NATS nkey pair for authentication", "", cmd)
	if err != nil {
		Logger.Fatal().Msg(err.Error())
	}
}
