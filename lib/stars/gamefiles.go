package stars

import (
	"encoding/base64"

	hs "github.com/neper-stars/houston"
	"github.com/neper-stars/neper/models"
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
// This is the only place where we set the Orders and Turns fields
// that will be used to send back to the client.
// This is due to the fact that we define a SessionFiles model with swagger that contains
// all fields
func (g GameFiles) HydrateSessionFilesDB(s *models.SessionFilesDB) error {
	header, err := hs.FileData(g.HostFile).FileHeader()
	if err != nil {
		return err
	}

	s.Year = int64(header.Year())
	s.Universe = b64encode(g.Universe)
	s.HostFile = b64encode(g.HostFile)

	var turns []*models.Turn
	var turnsDB []models.Turn
	for i := range g.Turns {
		turn := models.Turn{B64Data: b64encode(g.Turns[i])}
		turns = append(turns, &turn)
		turnsDB = append(turnsDB, turn)
	}
	s.Turns = turns
	s.TurnsDB = turnsDB

	var orders []*models.Order
	var ordersDB []models.Order
	for i := range g.Orders {
		order := models.Order{B64Data: b64encode(g.Orders[i])}
		orders = append(orders, &order)
		ordersDB = append(ordersDB, order)
	}
	s.Orders = orders
	s.OrdersDB = ordersDB
	return nil
}
