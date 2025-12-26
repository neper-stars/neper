package neper

import (
	"encoding/base64"
	"errors"
	"io"
	"os"

	hs "github.com/neper-stars/houston"

	"github.com/neper-stars/neper/lib/racefiles"
	"github.com/neper-stars/neper/models"
)

// RaceFileOptions is an alias for racefiles.ProcessorOptions for convenience.
type RaceFileOptions = racefiles.ProcessorOptions

// RaceAnalysis is an alias for racefiles.Analysis for convenience.
type RaceAnalysis = racefiles.Analysis

func RaceFromString(data string, opts RaceFileOptions) (*models.Race, *RaceAnalysis, error) {
	r := models.Race{}
	r.Data = data

	// first decode the base64 to binary
	rawData, err := r.RawData()
	if err != nil {
		return nil, nil, err
	}

	// Process race file (analyze, repair if enabled, strip password if enabled)
	processedData, analysis := racefiles.ProcessData(nil, rawData, opts)

	// Update the stored data if we modified it
	if analysis.WasRepaired || analysis.PasswordStripped {
		r.Data = base64.StdEncoding.EncodeToString(processedData)
	}

	// parse data
	if err := ParseRaceData(&r, processedData); err != nil {
		return nil, analysis, err
	}
	return &r, analysis, nil
}

func RaceFromFile(fn string, opts RaceFileOptions) (*models.Race, *RaceAnalysis, error) {
	f, err := os.Open(fn) // #nosec G304 -- filename comes from trusted source
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()
	return RaceFromReader(f, opts)
}

func RaceFromReader(r io.Reader, opts RaceFileOptions) (*models.Race, *RaceAnalysis, error) {
	rawData, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}

	// Process race file (analyze, repair if enabled, strip password if enabled)
	processedData, analysis := racefiles.ProcessData(nil, rawData, opts)

	race := models.RaceFromRaw(rawData)

	// Update data if modified
	if analysis.WasRepaired || analysis.PasswordStripped {
		race.Data = base64.StdEncoding.EncodeToString(processedData)
	}

	if err := ParseRaceData(race, processedData); err != nil {
		return nil, analysis, err
	}
	return race, analysis, nil
}

func ParseRaceData(race *models.Race, rawData []byte) error {
	// now start parsing binary data
	fd := hs.FileData(rawData)

	// get the blocks back
	bl, err := fd.BlockList()
	if err != nil {
		return err
	}

	// iterate on all blocks to find the player blocks
	// in a race file there should be only one player block
	for _, b := range bl {
		switch b.BlockTypeID() {
		case hs.PlayerBlockType:
			pb, ok := b.(hs.PlayerBlock)
			if !ok {
				return errors.New("failed to assert player block")
			}
			if pb.Valid {
				race.NamePlural = pb.NamePlural
				race.NameSingular = pb.NameSingular
			}
		// else
		// fmt.Println("empty player block... nothing to report")
		default:
		}
	}
	return nil
}
