package stars

import (
	"encoding/base64"
	"errors"

	hs "github.com/neper-stars/houston"
)

func RaceNamesFromHostFile(rawData []byte) ([]string, error) {
	// now start parsing binary data
	fd := hs.FileData(rawData)

	// get the blocks back
	bl, err := fd.BlockList()
	if err != nil {
		return nil, err
	}

	// iterate on all blocks to find the player blocks
	// in a host file there should be as many player blocks as in your session_player_race
	var playerRaceNames []string
	for _, b := range bl {
		switch b.BlockTypeID() {
		case hs.PlayerBlockType:
			pb, ok := b.(hs.PlayerBlock)
			if !ok {
				return nil, errors.New("failed to assert player block")
			}
			if pb.Valid {
				playerRaceNames = append(playerRaceNames, pb.NamePlural)
			}
		// else
		// fmt.Println("empty player block... nothing to report")
		default:
		}
	}
	return playerRaceNames, nil
}

func B64Decode(data string) ([]byte, error) {
	dst := make([]byte, base64.StdEncoding.DecodedLen(len(data)))
	n, err := base64.StdEncoding.Decode(dst, []byte(data))
	if err != nil {
		return nil, err
	}
	dst = dst[:n]
	return dst, nil
}
