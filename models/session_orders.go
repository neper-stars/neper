package models

import (
	"database/sql/driver"
	"errors"

	"github.com/jackc/pgtype"
)

// OrderList is a list of Order
type OrderList []Order

// Scan implements the database/sql Scanner interface.
// This loads data from the DB into our struct
func (a *OrderList) Scan(src interface{}) error {
	if src == nil {
		return errors.New("scan of Order: cannot scan NULL value")
	}

	var data pgtype.JSONB
	if err := data.Scan(src); err != nil {
		return err
	}

	if data.Status == pgtype.Null {
		return errors.New("scan of Order: cannot scan NULL value")
	}

	return data.AssignTo(a)
}

// Value implements the database/sql/driver Valuer interface.
// this dumps our struct into a database ready representation
func (a OrderList) Value() (driver.Value, error) {
	v := a
	if v == nil {
		v = OrderList{}
	}
	var data pgtype.JSONB
	if err := data.Set(v); err != nil {
		return nil, err
	}
	return data.Value()
}
