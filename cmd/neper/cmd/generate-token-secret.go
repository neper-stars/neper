package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// GenerateTokenSecretCmd ...
type GenerateTokenSecretCmd struct{}

// Execute ...
func (cmd *GenerateTokenSecretCmd) Execute([]string) error {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	fmt.Println("token-secret",
		hex.EncodeToString(secret))
	return nil
}

func init() {
	cmd := GenerateTokenSecretCmd{}

	_, err := parser.AddCommand("generate-token-secret", "generate an random token secret", "", &cmd)
	if err != nil {
		Logger.Fatal().Msg(err.Error())
	}
}
