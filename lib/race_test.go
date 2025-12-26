package neper

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type raceTestCase struct {
	Name                 string
	Filename             string
	ExpectedNameSingular string
}

func TestRaceFromFile(t *testing.T) {
	testCases := []raceTestCase{
		{
			"humans",
			"stars/fixtures/humans.r1",
			"Human",
		},
		{
			"halflings",
			"stars/fixtures/halflings.r1",
			"Halfing",
		},
		{
			"hobbits",
			"stars/fixtures/hobbits.r1",
			"Hobbit",
		},
	}
	for _, tCase := range testCases {
		t.Run(tCase.Name, func(t *testing.T) {
			r, _, err := RaceFromFile(tCase.Filename, RaceFileOptions{})
			require.NoError(t, err)
			require.Equal(t, tCase.ExpectedNameSingular, r.NameSingular)
		})
	}
}
