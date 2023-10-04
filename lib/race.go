package neper

import (
	"encoding/base64"
	"errors"

	hs "github.com/neper-stars/houston"

	"github.com/neper-stars/neper/models"
)

func unBase64(data string) ([]byte, error) {
	dst := make([]byte, base64.StdEncoding.DecodedLen(len(data)))
	n, err := base64.StdEncoding.Decode(dst, []byte(data))
	if err != nil {
		return nil, err
	}
	dst = dst[:n]
	return dst, nil
}

func NewRace(data string) (*models.Race, error) {
	r := models.Race{}
	r.Data = data

	// first decode the base64 to binary
	rawData, err := unBase64(data)
	if err != nil {
		return nil, err
	}

	// now start parsing binary data
	fd := hs.FileData(rawData)

	// get the blocks back
	bl, err := fd.BlockList()
	if err != nil {
		return nil, err
	}

	// iterate on all blocks to find the player blocks
	// in a race file there should be only one player block
	for _, b := range bl {
		switch b.BlockTypeID() {
		case hs.PlayerBlockType:
			pb, ok := b.(hs.PlayerBlock)
			if !ok {
				return nil, errors.New("failed to assert player block")
			}
			if pb.Valid {
				r.NamePlural = pb.NamePlural
				r.NameSingular = pb.NameSingular
			}
		// else
		// fmt.Println("empty player block... nothing to report")
		default:
		}
	}

	return &r, nil
}
