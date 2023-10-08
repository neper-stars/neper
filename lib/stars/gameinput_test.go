package stars

import (
	"testing"

	"github.com/stretchr/testify/require"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/rs/zerolog"
	"github.com/neper-stars/neper/models"
)

func testRuleset(t *testing.T) models.Ruleset {
	t.Helper()
	return models.Ruleset{
		Density:                          2,
		RandomSeed:                       0,
		StartingDistance:                 2,
		UniverseSize:                     2,
		VcAttainTechXInYField:            true,
		VcAttainTechXInYFieldTechValue:   26,
		VcAttainTechXInYFieldFieldsValue: 4,
		VcWinnerMustMeetxOfTheAbove:      1,
		VcAtLeastxYearsMustPassBeforeaWinnerIsDeclared: 100,
	}
}

func testPlayers(t *testing.T) []models.SessionPlayerRace {
	t.Helper()
	level1 := int64(1)
	return []models.SessionPlayerRace{
		{
			ID:            "spr1",
			PlayerOrder:   0,
			RaceID:        "hobbitsID",
			SessionID:     "shireID",
			UserProfileID: "merryID",
		},
		{
			ID:            "spr2",
			PlayerOrder:   1,
			RaceID:        "halflings",
			SessionID:     "shireID",
			UserProfileID: "gollumID",
		},
		{
			BotLevel:      &level1,
			ID:            "1",
			IsBot:         true,
			PlayerOrder:   3,
			RaceID:        "1",
			SessionID:     "shireID",
			UserProfileID: "system",
		},
	}
}

func testGameInput(t *testing.T, log zerolog.Logger) *GameInput {
	t.Helper()
	return NewGameInput(&log, "z:\\stars", "shireID", "The Shire", testRuleset(t), testPlayers(t))
}

var expectedFileContent = `The Shire
2 2 2 
0 0 0 0 0 0 0
3
z:\stars\shireID\game.r1
z:\stars\shireID\game.r2
# 1 1
0 
1 26 4
0 
0 
0 
0 
0 
1 100
shireID.xy`

func TestGameInput(t *testing.T) {
	log := testutils.GetLogger(t)
	gi := testGameInput(t, log)

	t.Run("computeRace", func(t *testing.T) {
		require.Equal(t, 3, len(gi.Players))
		require.Equal(t, 3, len(gi.Races))
		require.Equal(t, "z:\\stars\\shireID\\game.r1", gi.Races[0])
		require.Equal(t, "z:\\stars\\shireID\\game.r2", gi.Races[1])
		require.Equal(t, "# 1 1", gi.Races[2])
	})
	t.Run("fullFile", func(t *testing.T) {
		content, err := gi.Content()
		require.NoError(t, err)
		require.Equal(t, expectedFileContent, string(content))
	})
}
