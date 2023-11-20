package handlers

import (
	"net/http"
	"testing"

	"context"

	"os"

	"io"

	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"github.com/neper-stars/neper/fixtures"
	"github.com/neper-stars/neper/lib/sessionSubmitter"
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/models/types"
	"github.com/neper-stars/neper/restapi/operations"
	"github.com/neper-stars/neper/sync"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"
)

type wantsWebSocketTestCase struct {
	name         string
	input        http.Request
	expectedWant bool
}

func TestWantsWebSocket(t *testing.T) {
	tCases := []wantsWebSocketTestCase{
		{
			"needs",
			http.Request{
				Header: http.Header{
					"Connection": []string{"upgrade"},
					"Upgrade":    []string{"websocket"},
				},
			},
			true,
		},
		{
			"needs-badheader1",
			http.Request{
				Header: http.Header{
					"connection": []string{"upgrade"},
					"Upgrade":    []string{"websocket"},
				},
			},
			true,
		},
		{
			"needs-badheader2",
			http.Request{
				Header: http.Header{
					"connection": []string{"upGRade"},
					"Upgrade":    []string{"websocket"},
				},
			},
			true,
		},
		{
			"need-badheaders3",
			http.Request{
				Header: http.Header{
					"Connection": []string{"up", "grade"},
					"Upgrade":    []string{"websocket"},
				},
			},
			true,
		},
		{
			"noneed",
			http.Request{
				Header: http.Header{},
			},
			false,
		},
		{
			"noneed-fopapousser",
			http.Request{
				Header: http.Header{
					"connexion": []string{"upG-Rade"},
					"Upgrade":   []string{"websocket"},
				},
			},
			false,
		},
	}
	for _, tCase := range tCases {
		t.Run(tCase.name, func(t *testing.T) {
			resp := wantsWebSocket(&tCase.input)
			require.Equal(t, tCase.expectedWant, resp)
		})
	}
}

func TestSubmitTurn(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	// load merry vs gollum game
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum.json")
	// add some files so merry has already submitted its turn,
	// and we just need to submit a turn for gollum,
	// and see if we get our new turn back, out of the websocket
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")

	sessionID := "merryvsgollumID"
	gollumID := "gollumID"
	gollumTurn, err := os.Open("fixtures/merryvsgollum/Game.x2")
	require.NoError(t, err)
	gollumTurnData, err := io.ReadAll(gollumTurn)
	require.NoError(t, err)
	gollumTurnB64 := stars.B64Encode(gollumTurnData)
	gollumPrincipal := &models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   gollumID,
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
	}

	turnSubmitter := sessionSubmitter.NewTestSessionSubmitter(&log)

	handler := NewTurnSubmitHandler(&log, testdb.DB, turnSubmitter)
	// we cannot test the websocket upgrade in a unit test because we are not
	// at the right layer.
	// to test websockets you should go to the integration tests
	// which are located in the `tests` root dir of this project
	t.Run("gollum_submits_and_does_not_wait_new_turn", func(t *testing.T) {
		params := operations.TurnSubmitParams{
			SessionID: sessionID,
			Turnfile:  &types.Order{B64Data: gollumTurnB64},
			Year:      2400,
		}
		turnDetails, err := handler.handle(ctx, params, gollumPrincipal)
		require.NoError(t, err)
		// the server should return the same year as the file we just sent
		assert.Equal(t, 2400, turnDetails.Year)
		// gollum plays as second player
		assert.Equal(t, int64(1), turnDetails.PlayerOrder)
		// returned sessionID should be the same as our submission
		assert.Equal(t, sessionID, turnDetails.SessionID)
		// at least one message should have been sent to the turn generator
		assert.Equal(t, 1, len(turnSubmitter.Msgs))
	})
}
