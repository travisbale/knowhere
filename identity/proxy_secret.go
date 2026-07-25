package identity

import (
	"crypto/subtle"
	"net/http"
)

// ProxySecretHeader is the header a trusted edge (e.g. a Cloudflare Worker) sets to prove
// a request passed through it. RequireProxySecret validates it.
const ProxySecretHeader = "X-Proxy-Secret"

// RequireProxySecret rejects any request whose X-Proxy-Secret header does not match the
// configured secret, so a service only serves traffic that arrived through the trusted
// edge that injects the header. This keeps a directly-reachable origin (e.g. a Cloud Run
// *.run.app URL) from being hit around the edge — bounding cost and keeping bots off the
// database.
//
// Requests to any of exemptPaths are always allowed; pass the health-check path, which
// load balancers and container probes hit directly without going through the edge.
//
// An empty secret disables the check entirely and returns next unchanged — the expected
// configuration for local development, where nothing sets the header.
func RequireProxySecret(secret string, exemptPaths ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if secret == "" {
			return next
		}
		want := []byte(secret)
		exempt := make(map[string]struct{}, len(exemptPaths))
		for _, p := range exemptPaths {
			exempt[p] = struct{}{}
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := exempt[r.URL.Path]; !ok {
				got := []byte(r.Header.Get(ProxySecretHeader))
				if subtle.ConstantTimeCompare(got, want) != 1 {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
