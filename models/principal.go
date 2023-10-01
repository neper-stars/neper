package models

import (
	"github.com/rs/zerolog"
	"gopkg.in/dgrijalva/jwt-go.v3"
)

// Principal holds the current user principals that were extracted from
// the JWT and the database
type Principal struct {
	jwt.StandardClaims
	IsGlobalManager bool
	NickName        string
}

// MarshalZerologObject implements the zerolog.LogObjectMarshaler interface
func (p *Principal) MarshalZerologObject(e *zerolog.Event) {
	e.
		Str("id", p.Subject)
}

// NewPrincipal crafts a new principal and returns a pointer to it.
// this is used for tests
func NewPrincipal(subject string) *Principal {
	return &Principal{
		StandardClaims: jwt.StandardClaims{Subject: subject},
	}
}
