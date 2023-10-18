package handlers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type wantsWebSocketTestCase struct {
	name         string
	input        http.Request
	expectedWant bool
}

func TestWantsWebSocket(t *testing.T) {
	tCases := []wantsWebSocketTestCase{
		{
			"needs",
			http.Request{
				Header: http.Header{
					"Connection": []string{"upgrade"},
					"Upgrade":    []string{"websocket"},
				},
			},
			true,
		},
		{
			"needs-badheader1",
			http.Request{
				Header: http.Header{
					"connection": []string{"upgrade"},
					"Upgrade":    []string{"websocket"},
				},
			},
			true,
		},
		{
			"needs-badheader2",
			http.Request{
				Header: http.Header{
					"connection": []string{"upGRade"},
					"Upgrade":    []string{"websocket"},
				},
			},
			true,
		},
		{
			"need-badheaders3",
			http.Request{
				Header: http.Header{
					"Connection": []string{"up", "grade"},
					"Upgrade":    []string{"websocket"},
				},
			},
			true,
		},
		{
			"noneed",
			http.Request{
				Header: http.Header{},
			},
			false,
		},
		{
			"noneed-fopapousser",
			http.Request{
				Header: http.Header{
					"connexion": []string{"upG-Rade"},
					"Upgrade":   []string{"websocket"},
				},
			},
			false,
		},
	}
	for _, tCase := range tCases {
		t.Run(tCase.name, func(t *testing.T) {
			resp := wantsWebSocket(&tCase.input)
			require.Equal(t, tCase.expectedWant, resp)
		})
	}
}
