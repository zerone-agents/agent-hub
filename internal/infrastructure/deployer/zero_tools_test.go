package deployer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestZeroToolsCapabilityRequiresBothDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       bool
	}{
		{"supported", `{"success":true,"data":{"explicitZeroTools":true,"zeroToolsImageInspection":true}}`, 200, true},
		{"old", `{"success":true,"data":{"version":"3.1.0"}}`, 200, false},
		{"no image gate", `{"success":true,"data":{"explicitZeroTools":true}}`, 200, false},
		{"business failure", `{"success":false,"data":{"explicitZeroTools":true,"zeroToolsImageInspection":true}}`, 200, false},
		{"missing route", `{}`, 404, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/v1/capabilities", r.URL.Path)
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			ok, err := NewClient(srv.URL, "private-key").SupportsZeroTools(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.want, ok)
		})
	}
}
