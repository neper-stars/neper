package sync

import (
	"context"

	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/models"
)

func (w *Worker) syncRuleset(ctx context.Context, sql database.SQLHelper, op Operation, data *Ruleset) error {
	if op == OpCreate || op == OpUpdate {
		var rulesetDB = models.RulesetDB{
			ID:        data.Id,
			SessionID: data.SessionId,
			Ruleset: models.Ruleset{
				AcceleratedBbsPlay:           data.AcceleratedBbsPlay,
				ComputerPlayersFormAlliances: data.ComputerPlayersFormAlliances,
				Density:                      int64(data.Density),
				GalaxyClumping:               data.GalaxyClumping,
				MaximumMinerals:              data.MaximumMinerals,
				NoRandomEvents:               data.NoRandomEvents,
				PublicPlayerScores:           data.PublicPlayerScores,
				RandomSeed:                   int64(data.RandomSeed),
				SlowerTechAdvances:           data.SlowerTechAdvances,
				StartingDistance:             int64(data.StartingDistance),
				UniverseSize:                 int64(data.UniverseSize),
				VcAtLeastxYearsMustPassBeforeaWinnerIsDeclared: int64(data.VcAtLeastXYearsMustPassBeforeAWinnerIsDeclared),
				VcAttainTechXInYField:                          data.VcAttainTechXInYField,
				VcAttainTechXInYFieldFieldsValue:               int64(data.VcAttainTechXInYFieldFieldsValue),
				VcAttainTechXInYFieldTechValue:                 int64(data.VcAttainTechXInYFieldTechValue),
				VcExceedNextPlayerScoreByx:                     data.VcExceedNextPlayerScoreByX,
				VcExceedNextPlayerScoreByxValue:                int64(data.VcExceedNextPlayerScoreByXValue),
				VcExceedScoreOfx:                               data.VcExceedScoreOfX,
				VcExceedScoreOfxValue:                          int64(data.VcExceedScoreOfXValue),
				VcHasProductionCapacityOfxThousand:             data.VcHasProductionCapacityOfXThousand,
				VcHasProductionCapacityOfxThousandValue:        int64(data.VcHasProductionCapacityOfXThousandValue),
				VcHaveHighestScoreAfterxYears:                  data.VcHaveHighestScoreAfterXYears,
				VcHaveHighestScoreAfterxYearsValue:             int64(data.VcHaveHighestScoreAfterXYearsValue),
				VcOwnsxCapitalShips:                            data.VcOwnsXCapitalShips,
				VcOwnsxCapitalShipsValue:                       int64(data.VcOwnsXCapitalShipsValue),
				VcOwnsxPercentOfPlanets:                        data.VcOwnsXPercentOfPlanets,
				VcOwnsxPercentOfPlanetsValue:                   int64(data.VcOwnsXPercentOfPlanetsValue),
				VcWinnerMustMeetxOfTheAbove:                    int64(data.VcWinnerMustMeetXOfTheAbove),
			},
		}
		if err := sql.Upsert(&rulesetDB); err != nil {
			return err
		}
	}
	return nil
}
