package models

// SessionFilesDB stores the files for a session
// for a certain turn
// dbtable:"session_file" dbpkey:"id"
type SessionFilesDB struct {
	SessionFiles

	TurnsDB  TurnList  `db:"turns"`
	OrdersDB OrderList `db:"orders"`
}
