// Package reqctx carries per-request values (the raw request, the admin
// session, the authenticated agent) from middleware to strict handlers.
package reqctx

import (
	"context"
	"net"
	"net/http"
	"net/netip"

	"github.com/joshualent/lor/internal/auth"
	"github.com/joshualent/lor/internal/store"
)

type key int

const (
	requestKey key = iota
	sessionKey
	agentKey
)

// WithRequest stores the HTTP request.
func WithRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, requestKey, r)
}

// Request returns the HTTP request.
func Request(ctx context.Context) *http.Request {
	r, _ := ctx.Value(requestKey).(*http.Request)
	return r
}

// WithSession stores the admin session.
func WithSession(ctx context.Context, s *auth.SessionInfo) context.Context {
	return context.WithValue(ctx, sessionKey, s)
}

// Session returns the admin session (nil on public operations).
func Session(ctx context.Context) *auth.SessionInfo {
	s, _ := ctx.Value(sessionKey).(*auth.SessionInfo)
	return s
}

// WithAgent stores the authenticated agent.
func WithAgent(ctx context.Context, a *store.Agent) context.Context {
	return context.WithValue(ctx, agentKey, a)
}

// Agent returns the authenticated agent (nil on Enroll).
func Agent(ctx context.Context) *store.Agent {
	a, _ := ctx.Value(agentKey).(*store.Agent)
	return a
}

// RemoteIP is the TCP peer address. X-Forwarded-For is never used for
// authentication.
func RemoteIP(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return ip.Unmap()
}

// Meta returns information recorded with sessions and audit events.
func Meta(r *http.Request) auth.RequestMeta {
	return auth.RequestMeta{UserAgent: r.UserAgent(), ForwardedFor: r.Header.Get("X-Forwarded-For")}
}
