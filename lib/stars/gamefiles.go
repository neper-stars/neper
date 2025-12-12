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

func B64Encode(in []byte) string {
	dst := make([]byte, base64.StdEncoding.EncodedLen(len(in)))
	base64.StdEncoding.Encode(dst, in)
	return string(dst)
}

func NewGameFilesFromSessionFiles(sessionFiles models.SessionFiles) (*GameFiles, error) {
	var gf GameFiles
	universe, err := B64Decode(sessionFiles.Universe)
	if err != nil {
		return nil, err
	}
	gf.Universe = universe

	hostfile, err := B64Decode(sessionFiles.HostFile)
	if err != nil {
		return nil, err
	}
	gf.HostFile = hostfile

	turns, err := TurnListToBytes(sessionFiles.Turns)
	if err != nil {
		return nil, err
	}
	gf.Turns = turns

	orders, err := OrderListToBytes(sessionFiles.Orders)
	if err != nil {
		return nil, err
	}
	gf.Orders = orders

	return &gf, nil
}

func TurnListToBytes(tl types.TurnList) ([][]byte, error) {
	var res [][]byte
	for _, t := range tl {
		turn, err := B64Decode(t.B64Data)
		if err != nil {
			return nil, err
		}
		res = append(res, turn)
	}
	return res, nil
}

func OrderListToBytes(ol types.OrderList) ([][]byte, error) {
	var res [][]byte
	for _, o := range ol {
		order, err := B64Decode(o.B64Data)
		if err != nil {
			return nil, err
		}
		res = append(res, order)
	}
	return res, nil
}

// HydrateSessionFiles fills all the SessionFiles fields according to the
// current GameFiles
func (g GameFiles) HydrateSessionFiles(s *models.SessionFiles) error {
	header, err := hs.FileData(g.HostFile).FileHeader()
	if err != nil {
		return err
	}

	s.Year = int64(header.Year())
	s.Universe = B64Encode(g.Universe)
	s.HostFile = B64Encode(g.HostFile)

	var turns []types.Turn
	for i := range g.Turns {
		turns = append(turns, types.Turn{B64Data: B64Encode(g.Turns[i])})
	}
	s.Turns = turns

	var orders []types.Order
	for i := range g.Orders {
		orders = append(orders, types.Order{B64Data: B64Encode(g.Orders[i])})
	}
	s.Orders = orders
	return nil
}
