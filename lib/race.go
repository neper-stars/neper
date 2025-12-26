package neper

import (
	"encoding/base64"
	"errors"
	"io"
	"os"

	hs "github.com/neper-stars/houston"

	"github.com/neper-stars/neper/models"
)

// RaceFileOptions controls race file processing behavior
type RaceFileOptions struct {
	StripPassword bool // Remove password from race file
	FixCorrupted  bool // Repair corrupted race files (checksum issues)
}

// RaceAnalysis contains analysis results for a race file
type RaceAnalysis struct {
	NeedsRepair    bool   // True if the race file has corruption issues
	WasRepaired    bool   // True if the race file was repaired
	RepairError    string // Error message if repair was attempted but failed
	HasPassword    bool   // True if the race file has a password
	PasswordStripped bool // True if password was removed
}

func RaceFromString(data string, opts RaceFileOptions) (*models.Race, *RaceAnalysis, error) {
	r := models.Race{}
	r.Data = data
	analysis := &RaceAnalysis{}

	// first decode the base64 to binary
	rawData, err := r.RawData()
	if err != nil {
		return nil, nil, err
	}

	// Track if we modified the data
	modified := false

	// Always analyze race file for corruption issues
	rawData, modified, err = analyzeAndRepairRaceFile(rawData, opts.FixCorrupted, analysis)
	if err != nil {
		return nil, analysis, err
	}

	// Strip password from race file if enabled
	if opts.StripPassword {
		var stripped bool
		rawData, stripped, err = stripRacePassword(rawData)
		if err != nil {
			return nil, analysis, err
		}
		if stripped {
			analysis.PasswordStripped = true
			modified = true
		}
	}

	// Update the stored data if we modified it
	if modified {
		r.Data = base64.StdEncoding.EncodeToString(rawData)
	}

	// parse data
	if err := ParseRaceData(&r, rawData); err != nil {
		return nil, analysis, err
	}
	return &r, analysis, nil
}

// analyzeAndRepairRaceFile always analyzes a race file for corruption issues.
// If doRepair is true and the file needs repair, it attempts to fix it.
// Race files can become corrupted when manipulated in different Stars! installations.
// Returns the (possibly repaired) data, whether it was modified, and any error.
func analyzeAndRepairRaceFile(data []byte, doRepair bool, analysis *RaceAnalysis) ([]byte, bool, error) {
	info, err := hs.AnalyzeRaceFileBytes("", data)
	if err != nil {
		// Analysis failed, return original data without marking as needing repair
		return data, false, nil
	}

	// Always record if file needs repair (for warning purposes)
	analysis.NeedsRepair = info.NeedsRepair
	analysis.HasPassword = info.HasPassword

	if !info.NeedsRepair {
		return data, false, nil
	}

	// File needs repair - only attempt if enabled
	if !doRepair {
		return data, false, nil
	}

	// Attempt repair
	repaired, result, err := hs.RepairRaceFileBytes(data)
	if err != nil {
		analysis.RepairError = err.Error()
		return data, false, nil
	}
	if result.Success {
		analysis.WasRepaired = true
		return repaired, true, nil
	}

	// Repair failed
	analysis.RepairError = "repair was not successful"
	return data, false, nil
}

// stripRacePassword removes any password from race file data.
// Returns the (possibly modified) data, whether it was modified, and any error.
func stripRacePassword(data []byte) ([]byte, bool, error) {
	result, repairResult, err := hs.RemoveRacePasswordBytes(data)
	if err != nil {
		// Password removal failed, return original data
		return data, false, nil
	}
	if repairResult.Success && repairResult.PasswordRemoved {
		return result, true, nil
	}
	// No password was present or removal wasn't needed
	return data, false, nil
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

	analysis := &RaceAnalysis{}
	modified := false

	// Always analyze race file for corruption issues
	rawData, modified, err = analyzeAndRepairRaceFile(rawData, opts.FixCorrupted, analysis)
	if err != nil {
		return nil, analysis, err
	}

	// Strip password from race file if enabled
	if opts.StripPassword {
		var stripped bool
		rawData, stripped, err = stripRacePassword(rawData)
		if err != nil {
			return nil, analysis, err
		}
		if stripped {
			analysis.PasswordStripped = true
			modified = true
		}
	}

	race := models.RaceFromRaw(rawData)

	// Update data if modified
	if modified {
		race.Data = base64.StdEncoding.EncodeToString(rawData)
	}

	if err := ParseRaceData(race, rawData); err != nil {
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
