package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/auth"
	"github.com/neper-stars/neper/fixtures"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/sync"
)

func mustReadToken(t *testing.T, tokenString string) *models.Principal {
	t.Helper()
	claims, err := readToken(t, tokenString)
	if err != nil {
		t.Fatal(err)
	}
	return claims
}

func readToken(t *testing.T, tokenString string) (*models.Principal, error) {
	t.Helper()
	token, err := jwt.ParseWithClaims(
		tokenString,
		&models.Principal{},
		func(token *jwt.Token) (interface{}, error) {
			return []byte("test"), nil
		})

	if err != nil {
		return nil, err
	}

	if !assert.True(t, token.Valid) {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(*models.Principal)
	if !ok {
		return nil, fmt.Errorf("invalid claims")
	}

	return claims, nil
}

func callhandle(
	ctx context.Context,
	t *testing.T,
	handler *Authenticate,
	credentials *models.Credentials,
) *models.Principal {
	t.Helper()
	tokenString, err := handler.handle(ctx, credentials)

	if !assert.NoError(t, err) {
		t.FailNow()
	}

	return mustReadToken(t, tokenString)
}

func TestAuthenticateHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())

	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	w, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, w, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, w, "fixtures/gandalf.json")

	authImpl := auth.NewAuth(
		auth.TokenOptions{Secret: "74657374", Expiration: 5 * time.Minute},
		testdb.DB,
		time.Now,
		log,
	)
	defer authImpl.Close()

	handler := NewAuthenticateHandler(testdb.DB, authImpl)

	t.Run("apikeyGandalf", func(t *testing.T) {
		claims := callhandle(ctx, t, handler, &models.Credentials{
			Nickname: "GandalfTheGrey",
			Apikey:   "apikeyGandalf",
		})

		assert.Equal(t, claims.Subject, "gandalfID")
	})

	t.Run("invalid credentials", func(t *testing.T) {
		t.Run("unknown api key", func(t *testing.T) {
			tokenString, err := handler.handle(ctx, &models.Credentials{
				Apikey: "apikeyUnknown",
			})

			assert.Equal(t, "", tokenString)
			if assert.Error(t, err) {
				assert.Equal(t, "invalid credentials", err.Error())
			}
		})
	})
}
