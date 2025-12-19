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

// insertTestSerialKey inserts a test serial key into the database using the model
func insertTestSerialKey(t *testing.T, ctx context.Context, testDB *database.TestDB, key string) {
	t.Helper()
	log := testutils.GetLogger(t)
	tx, err := database.Begin(ctx, testDB.DB)
	require.NoError(t, err)
	defer tx.RollbackIfOpened(log)

	sqlH := database.NewSQLHelper(ctx, tx, log)
	serialKey := models.SerialKeyDB{Key: key}
	_, err = sqlH.Insert(&serialKey)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

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

func TestUserinfo_WithSerialKey(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testDB := database.GetTestDB(ctx, t, migration.Source)
	defer testDB.Close()

	// Insert a test serial key first (required due to FK constraint)
	testSerialKey := "TEST1234"
	insertTestSerialKey(t, ctx, testDB, testSerialKey)

	w, err := sync.NewWorker(testDB.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, w, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, w, "fixtures/gandalf_with_serial.json")

	handler := NewUserinfoHandler(testDB.DB)

	principal := &models.Principal{
		StandardClaims:  jwt.StandardClaims{Subject: "gandalfID"},
		IsGlobalManager: true,
	}

	info, err := handler.handle(ctx, principal)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "gandalfID", info.User.ID)
	assert.Equal(t, "GandalfTheGrey", info.User.Nickname)
	assert.Equal(t, testSerialKey, info.SerialKey)
}

func TestUserinfo_AutoAssignsSerialKey(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testDB := database.GetTestDB(ctx, t, migration.Source)
	defer testDB.Close()

	// Insert some available serial keys for auto-assignment
	availableKey := "AVAIL001"
	insertTestSerialKey(t, ctx, testDB, availableKey)

	w, err := sync.NewWorker(testDB.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, w, "fixtures/sessions.json")
	// Load gandalf WITHOUT a serial key
	fixtures.LoadFixtureFile(t, w, "fixtures/gandalf.json")

	handler := NewUserinfoHandler(testDB.DB)

	principal := &models.Principal{
		StandardClaims:  jwt.StandardClaims{Subject: "gandalfID"},
		IsGlobalManager: true,
	}

	// First call should auto-assign a serial key
	info, err := handler.handle(ctx, principal)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "gandalfID", info.User.ID)
	assert.Equal(t, availableKey, info.SerialKey, "serial key should be auto-assigned")

	// Verify the serial key is persisted in the database
	var serialKeyInDB string
	err = testDB.DB.GetContext(ctx, &serialKeyInDB,
		"SELECT serial_key FROM user_profile WHERE id = $1", "gandalfID")
	require.NoError(t, err)
	assert.Equal(t, availableKey, serialKeyInDB, "serial key should be persisted in database")

	// Second call should return the same serial key (not assign a new one)
	info2, err := handler.handle(ctx, principal)
	require.NoError(t, err)
	require.NotNil(t, info2)
	assert.Equal(t, availableKey, info2.SerialKey, "serial key should be the same on subsequent calls")
}

func TestUserinfo_NoAvailableSerialKeys(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testDB := database.GetTestDB(ctx, t, migration.Source)
	defer testDB.Close()

	// Don't insert any serial keys - simulate no available keys

	w, err := sync.NewWorker(testDB.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, w, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, w, "fixtures/gandalf.json")

	handler := NewUserinfoHandler(testDB.DB)

	principal := &models.Principal{
		StandardClaims:  jwt.StandardClaims{Subject: "gandalfID"},
		IsGlobalManager: true,
	}

	// Should still return userinfo, just without a serial key
	info, err := handler.handle(ctx, principal)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "gandalfID", info.User.ID)
	assert.Empty(t, info.SerialKey, "serial key should be empty when no keys available")
}
