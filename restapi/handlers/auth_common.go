package handlers

type RelationType uint

const (
	RelationCreate RelationType = iota
	RelationRead
	RelationUpdate
	RelationDelete
)
