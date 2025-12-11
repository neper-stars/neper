package auth

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
)

func TestAuth(t *testing.T) {
	t.Run("cache full log message", func(t *testing.T) {
		t.Run("small cache", func(t *testing.T) {
			buf := bytes.NewBuffer(nil)
			log := zerolog.New(buf).Level(zerolog.WarnLevel)
			ctx := log.WithContext(context.Background())
			testdb := database.GetTestDB(ctx, t, migration.Source)
			defer testdb.Close()

			auth := NewAuth(TokenOptions{
				Secret:          "74657374",
				Expiration:      time.Minute,
				CacheMaxSize:    10,
				CachePurgeDelay: time.Minute,
			}, testdb.DB, time.Now, log)
			defer auth.Close()

			for _, id := range []string{
				"1", "2", "3",
			} {
				_, err := auth.MakeToken(models.Principal{
					StandardClaims: jwt.StandardClaims{
						Subject: id,
					},
				})
				require.NoError(t, err)
			}

			time.Sleep(time.Second)

			for _, id := range []string{
				"1", "2", "3",
				"4", "5", "6",
				"7", "8", "9",
				"10", "11", "12",
			} {
				_, err := auth.MakeToken(models.Principal{
					StandardClaims: jwt.StandardClaims{
						Subject: id,
					},
				})
				require.NoError(t, err)
			}

			assert.Equal(t, 10, auth.cache.ItemCount())
			assert.JSONEq(t, `{
				"level":"warn",
				"counters":{
					"total":10,
					"users":{
						"1":2,
						"2":2,
						"3":2,
						"4":1,
						"5":1,
						"6":1,
						"7":1
					}
				},
				"message":"auth token cache is full"
			}`, buf.String())
		})
	})
}
