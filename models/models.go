package models

import (
	"time"

	// this is a fix for the model generation that needs
	// this module and cannot declare it in our go.mod
	_ "github.com/fatih/structtag"
	"github.com/go-openapi/strfmt"
)

//go:generate go run ../dependencies/go-orusapi/scripts/generate_db_helpers.go

func init() {
	strfmt.MarshalFormat = strfmt.RFC3339Millis
	// always send datetimes as UTC to the API
	strfmt.NormalizeTimeForMarshal = func(t time.Time) time.Time {
		return t.UTC()
	}
}
