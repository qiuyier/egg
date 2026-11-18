package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qiuyier/egg/internal/config"
	"go.uber.org/zap"
)

func TestCORS(t *testing.T) {
	const allowed = "https://frontend.example"
	settings := config.CORS{
		Enabled:          true,
		AllowOrigins:     []string{allowed},
		AllowMethods:     []string{http.MethodGet},
		AllowHeaders:     []string{"Authorization"},
		AllowCredentials: true,
		ExposeHeaders:    []string{"X-Request-ID"},
	}
	for _, tc := range []struct {
		name, method, origin, responseOrigin string
		enabled                              bool
		status                               int
	}{
		{"disabled", http.MethodGet, allowed, "", false, http.StatusOK},
		{"allowed", http.MethodGet, allowed, allowed, true, http.StatusOK},
		{"denied", http.MethodGet, "https://other.example", "", true, http.StatusForbidden},
		{"preflight", http.MethodOptions, allowed, allowed, true, http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{CORS: settings}
			cfg.CORS.Enabled = tc.enabled
			router, err := New(cfg, zap.NewNop(), nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(tc.method, "/api/v1/ping", nil)
			req.Header.Set("Origin", tc.origin)
			if tc.method == http.MethodOptions {
				req.Header.Set("Access-Control-Request-Method", http.MethodGet)
				req.Header.Set("Access-Control-Request-Headers", "authorization")
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.status || rec.Header().Get("Access-Control-Allow-Origin") != tc.responseOrigin {
				t.Fatalf("unexpected CORS response: %d %v", rec.Code, rec.Header())
			}
			if tc.method == http.MethodOptions && (rec.Header().Get("Access-Control-Allow-Methods") != http.MethodGet ||
				!strings.EqualFold(rec.Header().Get("Access-Control-Allow-Headers"), "Authorization")) {
				t.Fatalf("preflight was missing methods or headers: %v", rec.Header())
			}
			if tc.responseOrigin != "" && rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
				t.Fatal("missing credentials header")
			}
			if tc.status == http.StatusOK && tc.enabled && !strings.EqualFold(rec.Header().Get("Access-Control-Expose-Headers"), "X-Request-ID") {
				t.Fatal("request ID was not exposed")
			}
		})
	}
}
