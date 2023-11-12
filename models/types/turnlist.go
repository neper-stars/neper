package types

import (
	"context"
	"database/sql/driver"
	"errors"

	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/jackc/pgtype"
)

// TurnList is a list of Turn in database
type TurnList []Turn

// Scan implements the database/sql Scanner interface.
// This loads data from the DB into our struct
func (a *TurnList) Scan(src interface{}) error {
	if src == nil {
		return errors.New("scan of Turn: cannot scan NULL value")
	}

	var data pgtype.JSONB
	if err := data.Scan(src); err != nil {
		return err
	}

	if data.Status == pgtype.Null {
		return errors.New("scan of Turn: cannot scan NULL value")
	}

	return data.AssignTo(a)
}

// Value implements the database/sql/driver Valuer interface.
// this dumps our struct into a database ready representation
func (a TurnList) Value() (driver.Value, error) {
	v := a
	if v == nil {
		v = TurnList{}
	}
	var data pgtype.JSONB
	if err := data.Set(v); err != nil {
		return nil, err
	}
	return data.Value()
}

func (a *TurnList) Validate(formats strfmt.Registry) error {
	for _, turn := range *a {
		if err := turn.Validate(formats); err != nil {
			return err
		}
	}
	return nil
}

// ContextValidate validates this turnlist based on context it is used
func (a *TurnList) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (a *TurnList) MarshalBinary() ([]byte, error) {
	if a == nil {
		return nil, nil
	}
	return swag.WriteJSON(a)
}

// UnmarshalBinary interface implementation
func (a *TurnList) UnmarshalBinary(b []byte) error {
	var res TurnList
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*a = res
	return nil
}
