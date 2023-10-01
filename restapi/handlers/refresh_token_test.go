package handlers

import (
	"context"
	"errors"
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

func TestRefreshHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	authSource := auth.NewAuth(
		auth.TokenOptions{Secret: "74657374", Expiration: 5 * time.Minute},
		testdb.DB,
		time.Now,
		log,
	)
	defer authSource.Close()
	handler := NewRefreshTokenHandler(&log, testdb.DB, authSource)

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")
	// gollum is not active
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gollum_nosession.json")

	token, err := handler.handle(ctx, &models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   "merryID",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
	})

	// all should work for merry
	require.NoError(t, err)
	claims := mustReadToken(t, token)
	assert.Equal(t, "merryID", claims.Subject)

	_, err = handler.handle(ctx, &models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   "gollumID",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
	})
	// gollum should be rejected, with auth.ErrInvalidCredentials
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrInvalidCredentials))
}
