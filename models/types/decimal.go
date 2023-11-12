package types

import (
	"context"

	"github.com/cockroachdb/apd/v3"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/shopspring/decimal"
)

type Decimal struct {
	apd.Decimal
}

func NewDecimal(coeff int64, exponent int32) *Decimal {
	v := apd.New(coeff, exponent)
	return &Decimal{*v}
}

func MustNewDecimalFromFloat64(value float64) Decimal {
	d := Decimal{}
	if _, err := d.SetFloat64(value); err != nil {
		panic(err)
	}
	return d
}

func MustNewDecimalFromInt64(value int64) Decimal {
	d := Decimal{}
	d.SetInt64(value)
	return d
}

func NewDecimalFromDecimal(d decimal.Decimal) Decimal {
	v, _, _ := apd.NewFromString(d.String())
	return Decimal{*v}
}

func NewDecimalFromString(s string) (*Decimal, error) {
	v, _, err := apd.NewFromString(s)
	if err != nil {
		return nil, err
	}
	return &Decimal{*v}, nil
}

func MustNewDecimalFromString(s string) Decimal {
	d, err := NewDecimalFromString(s)
	if err != nil {
		panic(err)
	}
	return *d
}

// ToShopspringDecimal converts the decimal to a shopspring/decimal
func (d Decimal) ToShopspringDecimal() decimal.Decimal {
	v, err := decimal.NewFromString(d.String())
	if err != nil {
		panic(err)
	}
	return v
}

func (d *Decimal) UnmarshalJSON(data []byte) error {
	if data[0] == '"' && data[len(data)-1] == '"' {
		return d.UnmarshalText(data[1 : len(data)-1])
	}
	return d.UnmarshalText(data)
}

func (d Decimal) MarshalJSON() ([]byte, error) {
	return d.MarshalText()
}

// Validate validates this decimal
func (d *Decimal) Validate(formats strfmt.Registry) error {
	return nil
}

// ContextValidate validates this decimal
func (d *Decimal) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (d *Decimal) MarshalBinary() ([]byte, error) {
	if d == nil {
		return nil, nil
	}
	return swag.WriteJSON(d)
}

// UnmarshalBinary interface implementation
func (d *Decimal) UnmarshalBinary(b []byte) error {
	var res Decimal
	if err := d.UnmarshalJSON(b); err != nil {
		return err
	}
	*d = res
	return nil
}

// MarshalYAML ...
func (d Decimal) MarshalYAML() (interface{}, error) {
	return d.String(), nil
}

func (d *Decimal) Cmp(o *Decimal) int {
	return d.Decimal.Cmp(&o.Decimal)
}
