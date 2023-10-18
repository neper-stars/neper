package models

// SessionFilesDB stores the files for a session
// for a certain turn
// dbtable:"session_file" dbpkey:"id"
type SessionFilesDB struct {
	SessionFiles

	TurnsDB  TurnList  `db:"turns"`
	OrdersDB OrderList `db:"orders"`
}

type ReadyPlayers []bool

// ReadyPlayers returns a ReadyPlayers struct that tell which player is ready of not
// this is sorted in player order ASC, you still need to wort which players are
// human if you want to know if you can generate a turn
func (s *SessionFilesDB) ReadyPlayers() ReadyPlayers {
	r := ReadyPlayers{}
	for _, order := range s.OrdersDB {
		r = append(r, order.B64Data != "")
	}
	return r
}

func (s *SessionFilesDB) ToTurnFiles(playerOrder int64) TurnFiles {
	var turnFiles TurnFiles
	var playerTurn PlayerTurn
	playerTurn.Turn = s.Turns[playerOrder].B64Data
	playerTurn.Universe = s.Universe
	turnFiles.Turn = &playerTurn
	turnFiles.Year = s.Year
	turnFiles.SessionID = s.SessionID
	return turnFiles
}
