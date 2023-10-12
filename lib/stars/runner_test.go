package stars

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"orus.io/orus-io/go-orusapi/testutils"

	neper "github.com/neper-stars/neper/lib"
	"github.com/neper-stars/neper/models"
)

func GetTestRaces(t *testing.T) []models.Race {
	t.Helper()
	var races []models.Race
	hobbits, err := neper.RaceFromFile("fixtures/hobbits.r1")
	require.NoError(t, err)
	hobbits.ID = "hobbitsID"
	races = append(races, *hobbits)

	halflings, err := neper.RaceFromFile("fixtures/halflings.r1")
	require.NoError(t, err)
	halflings.ID = "halflingsID"
	races = append(races, *halflings)
	return races
}

func TestRunner(t *testing.T) {
	log := testutils.GetLogger(t)
	gi := testGameInput(t, log)

	ctx := context.Background()

	runner, shutdown := GetTestStarsRunner(t, &log)
	defer shutdown()

	sessionID := "shireID"
	gameFiles, err := runner.NewGame(ctx, &log, sessionID, gi, testPlayers(t), GetTestRaces(t))
	require.NoError(t, err)
	require.Equal(t, 2, len(gameFiles.Turns))
}
