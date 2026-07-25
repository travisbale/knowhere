package identity

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequireProxySecret(t *testing.T) {
	const secret = "s3cr3t"

	tests := []struct {
		name       string
		secret     string
		path       string
		header     string
		wantStatus int
	}{
		{"empty secret disables the check", "", "/v1/anything", "", http.StatusOK},
		{"matching header passes", secret, "/v1/anything", secret, http.StatusOK},
		{"missing header is forbidden", secret, "/v1/anything", "", http.StatusForbidden},
		{"wrong header is forbidden", secret, "/v1/anything", "nope", http.StatusForbidden},
		{"exempt path is allowed without header", secret, "/healthz", "", http.StatusOK},
		{"exempt path still passes with header", secret, "/healthz", secret, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := RequireProxySecret(tt.secret, "/healthz")(okHandler())

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.header != "" {
				req.Header.Set(ProxySecretHeader, tt.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
