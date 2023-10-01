// This file is generated only once and is safe to edit

package cmd

import (
	"github.com/go-openapi/swag"

	"github.com/neper-stars/neper/auth"
	"github.com/neper-stars/neper/restapi"
)

var (
	// TokenOptions is the authentication token options
	TokenOptions = auth.NewTokenOptions()
)

func setupServerCmd(cmd *ServeCmd) {
	cmd.API.CommandLineOptionsGroups = append(
		cmd.API.CommandLineOptionsGroups,
		swag.CommandLineOptionsGroup{
			ShortDescription: "auth",
			LongDescription:  "authentication settings",
			Options:          TokenOptions,
		},
	)
}

func setupServeConfig(config *restapi.Config) error {
	config.BaseURL = InfoOptions.BaseURL
	// This is where the api config can be customized at will
	config.TokenOptions = *TokenOptions
	config.Authorizer = auth.NewAuthorizer(config.Log, config.DB)
	return nil
}
