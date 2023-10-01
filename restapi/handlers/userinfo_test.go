package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/fixtures"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/sync"
)

func TestUserinfo(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testDB := database.GetTestDB(ctx, t, migration.Source)
	defer testDB.Close()

	w, err := sync.NewWorker(testDB.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, w, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, w, "fixtures/gandalf.json")

	handler := NewUserinfoHandler(testDB.DB)

	for _, tt := range []struct {
		name      string
		principal *models.Principal
		info      *models.Userinfo
		err       error
	}{
		{"gandalf",
			&models.Principal{
				StandardClaims:  jwt.StandardClaims{Subject: "gandalfID"},
				IsGlobalManager: true},
			&models.Userinfo{
				User: &models.User{
					ID:       "gandalfID",
					Nickname: "GandalfTheGrey",
				},
			},
			nil,
		},
		{"unknown_user_returns_error",
			&models.Principal{
				StandardClaims:  jwt.StandardClaims{Subject: "unknown"},
				IsGlobalManager: false},
			nil,
			ErrUnknownUser,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info, err := handler.handle(ctx, tt.principal)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.info, info)
		})
	}
}
