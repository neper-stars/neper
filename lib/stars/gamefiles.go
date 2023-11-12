package stars

import (
	"encoding/base64"

	hs "github.com/neper-stars/houston"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/models/types"
)

// GameFiles is the struct that will be returned by
// Runner.NewGame
type GameFiles struct {
	Universe []byte   // one game.xy file (the universe and rules, everyone should receive this one)
	HostFile []byte   // one game.hst file (this is the host control file, only for Neper)
	Turns    [][]byte // one game.mX file for each player (including computer players) X should be the player number +1
	Orders   [][]byte // one .rX file for each player (only for the non computer players) X should be the player number +1
}

func b64encode(in []byte) string {
	dst := make([]byte, base64.StdEncoding.EncodedLen(len(in)))
	base64.StdEncoding.Encode(dst, in)
	return string(dst)
}

// HydrateSessionFilesDB fills all the SessionFilesDB fields according to the
// current GameFiles
func (g GameFiles) HydrateSessionFilesDB(s *models.SessionFilesDB) error {
	header, err := hs.FileData(g.HostFile).FileHeader()
	if err != nil {
		return err
	}

	s.Year = int64(header.Year())
	s.Universe = b64encode(g.Universe)
	s.HostFile = b64encode(g.HostFile)

	var turns []types.Turn
	for i := range g.Turns {
		turns = append(turns, types.Turn{B64Data: b64encode(g.Turns[i])})
	}
	s.Turns = turns

	var orders []types.Order
	for i := range g.Orders {
		orders = append(orders, types.Order{B64Data: b64encode(g.Orders[i])})
	}
	s.Orders = orders
	return nil
}
