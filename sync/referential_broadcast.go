package sync

import (
	"context"
	"fmt"
	"io"

	jsoniter "github.com/json-iterator/go"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"
)

func guessDataType(data []byte) (datatype DataTypeEnum, err error) {
	iter := jsoniter.ConfigDefault.BorrowIterator(data)
	defer jsoniter.ConfigDefault.ReturnIterator(iter)

	iter.ReadObjectCB(func(iter *jsoniter.Iterator, fieldName string) bool {
		if fieldName != "__type__" {
			iter.Skip()
			return true
		}
		value := iter.ReadString()
		if iter.Error != nil {
			return false
		}
		switch value {
		case "session":
			datatype = DataTypeEnumSession
		case "session_files":
			datatype = DataTypeEnumSessionFiles
		case "race":
			datatype = DataTypeEnumRace
		case "user_profile":
			datatype = DataTypeEnumUserProfile
		case "invitation":
			datatype = DataTypeEnumInvitation
		case "session_player_race":
			datatype = DataTypeEnumSessionPlayerRace
		case "ruleset":
			datatype = DataTypeEnumRuleset
		default:
			iter.ReportError("guessDataType", "unknown __type__: "+value)
			return false
		}
		return false
	})
	err = iter.Error
	return
}

func readChange(
	callback func(Operation, DataTypeEnum, []byte) error,
) func(*jsoniter.Iterator) bool {
	return func(iter *jsoniter.Iterator) bool {
		var (
			op       Operation
			datatype DataTypeEnum
			data     []byte
		)
		if !iter.ReadObjectCB(func(iter *jsoniter.Iterator, fieldname string) bool {
			switch fieldname {
			case "operation":
				switch value := iter.ReadString(); value {
				case "create":
					op = OpCreate
				case "update":
					op = OpUpdate
				case "no_change":
					op = OpNoChange
				default:
					if iter.Error == nil {
						iter.ReportError("handleNeperReferentialBroadcast",
							"Unknown operation: "+value)
					}
					return false
				}
			case "data":
				if next := iter.WhatIsNext(); next != jsoniter.ObjectValue {
					iter.ReportError("handleNeperReferentialBroadcast", "data: expects an object")
					return false
				}
				data = iter.SkipAndReturnBytes()
				dt, err := guessDataType(data)
				if err != nil {
					iter.ReportError("handleNeperReferentialBroadcast", err.Error())
					return false
				}
				datatype = dt
			}
			return true
		}) {
			return false
		}
		if err := callback(op, datatype, data); err != nil {
			iter.ReportError("readChange", err.Error())
		}
		return true
	}
}

func (w *Worker) handleNeperReferentialBroadcast(ctx context.Context, data io.Reader, log zerolog.Logger) error {
	log.Debug().Msg("handling a neper-referential-broadcast message")
	tx, err := database.Begin(ctx, w.db)
	if err != nil {
		return err
	}
	defer tx.RollbackIfOpened(log)

	sql := database.NewSQLHelper(ctx, tx, log)

	iter := jsoniter.ConfigDefault.BorrowIterator(make([]byte, 5*1024))
	iter.Reset(data)
	defer jsoniter.ConfigDefault.ReturnIterator(iter)

	iter.ReadObjectCB(func(iter *jsoniter.Iterator, fieldname string) bool {
		switch fieldname {
		case "version":
			_ = iter.ReadUint64()
			if iter.Error != nil {
				return false
			}
			return true
		case "changes":
			return iter.ReadArrayCB(
				readChange(func(op Operation, datatype DataTypeEnum, buf []byte) error {
					switch datatype {
					case DataTypeEnumSession:
						var data Session
						if err := data.UnmarshalJSON(buf); err != nil {
							return err
						}
						return w.syncSession(ctx, sql, op, &data)
					case DataTypeEnumRace:
						var data Race
						if err := data.UnmarshalJSON(buf); err != nil {
							return err
						}
						return w.syncRace(ctx, sql, op, &data)
					case DataTypeEnumUserProfile:
						var data UserProfile
						if err := data.UnmarshalJSON(buf); err != nil {
							return err
						}
						return w.syncUserProfile(ctx, sql, op, &data)
					case DataTypeEnumSessionPlayerRace:
						var data SessionPlayerRace
						if err := data.UnmarshalJSON(buf); err != nil {
							return err
						}
						return w.syncSessionPlayerRace(ctx, sql, op, &data)
					case DataTypeEnumSessionFiles:
						var data SessionFiles
						if err := data.UnmarshalJSON(buf); err != nil {
							return err
						}
						return w.syncSessionFiles(ctx, sql, op, &data)
					case DataTypeEnumRuleset:
						var data Ruleset
						if err := data.UnmarshalJSON(buf); err != nil {
							return err
						}
						return w.syncRuleset(ctx, sql, op, &data)
					default:
						return fmt.Errorf("unknown data type: %d", datatype)
					}
				}))
		default:
			iter.ReportError("handleNeperReferentialBroadcast", "Unknown property: "+fieldname)
			return false
		}
	})
	if iter.Error != nil {
		return iter.Error
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	log.Debug().Msg("successfully handled the referential-broadcast message")
	return nil
}
