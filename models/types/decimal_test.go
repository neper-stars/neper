package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecimal(t *testing.T) {
	t.Run("NewDecimalFromString", func(t *testing.T) {
		for _, tt := range []struct {
			name     string
			s        string
			expected *Decimal
			err      string
		}{
			{"valid", "0.01", NewDecimal(1, -2), ""},
			{"negative", "-12.01", NewDecimal(-1201, -2), ""},
			{"invalid", "0,01", nil, "parse mantissa: 0,01"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				r, err := NewDecimalFromString(tt.s)
				if tt.expected != nil {
					assert.Nil(t, err)
					require.NotNil(t, r)
					assert.Equal(t, *tt.expected, *r)
				} else {
					assert.Nil(t, r)
					require.EqualError(t, err, tt.err)
				}
			})
		}
	})

	t.Run("UnmarshallJSON", func(t *testing.T) {
		for _, tt := range []struct {
			name     string
			s        string
			expected *Decimal
			err      string
		}{
			{"number", `0.01`, NewDecimal(1, -2), ""},
			{"string", `"0.01"`, NewDecimal(1, -2), ""},
		} {
			t.Run(tt.name, func(t *testing.T) {
				var r Decimal
				err := json.Unmarshal([]byte(tt.s), &r)
				if tt.err == "" {
					assert.NoError(t, err)
					assert.Equal(t, *tt.expected, r)
				} else {
					assert.EqualError(t, err, tt.err)
				}
			})
		}
	})
}
