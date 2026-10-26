package server

import (
	"mime"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"

	"github.com/joshualent/lor/internal/apierr"
	"github.com/joshualent/lor/internal/server/reqctx"
)

const maxBodyBytes = 1 << 20

// securityHeaders sets the headers every admin-listener response carries.
// Lor never sends CORS headers.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// onlyFrom rejects requests whose TCP peer isn't allowed. The listener
// already drops such connections; this is defense in depth.
func onlyFrom(allowed netip.Addr, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reqctx.RemoteIP(r) != allowed {
			apierr.Write(w, apierr.Forbidden("not_from_proxy", "the admin listener only accepts requests from the trusted proxy"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// csrf enforces the plan's rules on every state-changing admin API request:
// Origin equal to public_url, X-Lor-Request: 1, Content-Type: application/json.
func csrf(origins []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !safeMethod(r.Method) {
			if o := r.Header.Get("Origin"); o == "" || !slices.Contains(origins, o) {
				apierr.Write(w, apierr.Forbidden("csrf_origin", "missing or wrong Origin header"))
				return
			}
			if r.Header.Get("X-Lor-Request") != "1" {
				apierr.Write(w, apierr.Forbidden("csrf_header", "missing X-Lor-Request header"))
				return
			}
			mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mt != "application/json" {
				apierr.Write(w, apierr.Forbidden("csrf_content_type", "Content-Type must be application/json"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// apiCommon sets no-store and limits body size on API routes.
func apiCommon(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

// jsonNotFound is the catch-all for unknown API paths: never index.html.
func jsonNotFound(w http.ResponseWriter, r *http.Request) {
	apierr.Write(w, apierr.NotFound("not_found", "no such API endpoint: %s %s", r.Method, r.URL.Path))
}

// filterListener closes connections whose peer address isn't allowed.
type filterListener struct {
	net.Listener
	allow func(netip.Addr) bool
}

func (l filterListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		ap, err := netip.ParseAddrPort(c.RemoteAddr().String())
		if err == nil && l.allow(ap.Addr().Unmap()) {
			return c, nil
		}
		c.Close()
	}
}

func hasPrefix(p, prefix string) bool {
	return p == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(p, prefix)
}
