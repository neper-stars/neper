package neper

import (
	"errors"
	"io"
	"os"

	hs "github.com/neper-stars/houston"

	"github.com/neper-stars/neper/models"
)

func RaceFromString(data string) (*models.Race, error) {
	r := models.Race{}
	r.Data = data

	// first decode the base64 to binary
	rawData, err := r.RawData()
	if err != nil {
		return nil, err
	}

	// parse data
	if err := ParseRaceData(&r, rawData); err != nil {
		return nil, err
	}
	return &r, nil
}

func RaceFromFile(fn string) (*models.Race, error) {
	f, err := os.Open(fn) // #nosec G304 -- filename comes from trusted source
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return RaceFromReader(f)
}

func RaceFromReader(r io.Reader) (*models.Race, error) {
	rawData, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	race := models.RaceFromRaw(rawData)

	if err := ParseRaceData(race, rawData); err != nil {
		return nil, err
	}
	return race, nil
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
