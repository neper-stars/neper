package models

import (
	"time"

	// this is a fix for the model generation that needs
	// this module and cannot declare it in our go.mod
	_ "github.com/fatih/structtag"
	"github.com/go-openapi/strfmt"
)

//go:generate go tool orus.io/orus-io/go-orusapi/tools/generate_db_helpers

func init() {
	strfmt.MarshalFormat = strfmt.RFC3339Millis
	// always send datetimes as UTC to the API
	strfmt.NormalizeTimeForMarshal = func(t time.Time) time.Time {
		return t.UTC()
	}
}
