package stars

import (
	"testing"

	"github.com/stretchr/testify/require"
	"orus.io/orus-io/go-orusapi/testutils"

	"fmt"

	"github.com/neper-stars/neper/models"
)

func TestGameInput_computeRace(t *testing.T) {
	log := testutils.GetLogger(t)
	ruleset := models.Ruleset{
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
	level1 := int64(1)
	players := []models.SessionPlayerRace{
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
	gi := NewGameInput(&log, "z:\\stars", "shireID", "The Shire", ruleset, players)

	fmt.Printf("%+v\n", gi.Races)
	require.Equal(t, 3, len(gi.Players))
	require.Equal(t, 3, len(gi.Races))
	require.Equal(t, "z:\\stars\\shireID\\game.r1", gi.Races[0])
	require.Equal(t, "z:\\stars\\shireID\\game.r2", gi.Races[1])
	require.Equal(t, "# 1 1", gi.Races[2])
}
