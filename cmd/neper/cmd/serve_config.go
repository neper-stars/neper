// This file is generated only once and is safe to edit

package cmd

import (
	"github.com/go-openapi/swag"

	"path/filepath"

	"github.com/neper-stars/neper/auth"
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/restapi"
)

var (
	// TokenOptions is the authentication token options
	TokenOptions       = auth.NewTokenOptions()
	StarsRunnerOptions = stars.NewRunnerOptions()
)

func setupServerCmd(cmd *ServeCmd) {
	cmd.API.CommandLineOptionsGroups = append(
		cmd.API.CommandLineOptionsGroups,
		swag.CommandLineOptionsGroup{
			ShortDescription: "auth",
			LongDescription:  "authentication settings",
			Options:          TokenOptions,
		},
		swag.CommandLineOptionsGroup{
			ShortDescription: "runner",
			LongDescription:  "stars runner",
			Options:          StarsRunnerOptions,
		},
	)
}

func setupServeConfig(config *restapi.Config) error {
	config.BaseURL = InfoOptions.BaseURL
	// This is where the api config can be customized at will
	config.TokenOptions = *TokenOptions
	absExecutableDir, err := filepath.Abs(StarsRunnerOptions.ExecutableDir)
	if err != nil {
		return err
	}
	absSaveDir, err := filepath.Abs(StarsRunnerOptions.SaveDir)
	if err != nil {
		return err
	}
	config.StarsRunner = stars.NewRunner(&config.Log, absExecutableDir, absSaveDir)
	if err := config.StarsRunner.InitialChecks(); err != nil {
		return err
	}
	return nil
}
