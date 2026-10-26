// Package server wires Lor's two listeners (plan sections 2, 5 and 11):
//
//   - admin (admin_listen): web UI + admin API, TLS, only from trusted_proxy
//   - agent (agent_listen): agent API, TLS (pinned by the CLI), bearer token
//   - source-IP check
//
// Admin routes exist only on the admin listener and agent routes only on the
// agent listener.
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/joshualent/lor/internal/apierr"
	"github.com/joshualent/lor/internal/auth"
	"github.com/joshualent/lor/internal/ca"
	"github.com/joshualent/lor/internal/config"
	"github.com/joshualent/lor/internal/grants"
	"github.com/joshualent/lor/internal/hosts"
	"github.com/joshualent/lor/internal/server/adminapi"
	"github.com/joshualent/lor/internal/server/agentapi"
	"github.com/joshualent/lor/internal/store"
)

// Options are the server's dependencies.
type Options struct {
	Config         *config.Config
	DB             *store.DB
	CA             *ca.CA
	Grants         *grants.Service
	Hosts          *hosts.Service
	Admin          *auth.Admin
	Agents         *auth.Agents
	TLS            tls.Certificate
	TLSFingerprint string
	Web            fs.FS // the embedded SPA build
	Logger         *slog.Logger
}

// Server holds both HTTP handlers.
type Server struct {
	opts  Options
	admin http.Handler
	agent http.Handler
	log   *slog.Logger
}

// New builds the handlers.
func New(o Options) (*Server, error) {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	s := &Server{opts: o, log: o.Logger}
	spa, err := newSPA(o.Web)
	if err != nil {
		return nil, err
	}

	// Admin listener: /api/admin/v1/*, /bootstrap/{name}.sh, SPA.
	ah := &adminapi.Handlers{Config: o.Config, DB: o.DB, Grants: o.Grants, Hosts: o.Hosts, Admin: o.Admin, Agents: o.Agents, TLSFingerprint: o.TLSFingerprint}
	adminStrict := adminapi.NewStrictHandlerWithOptions(ah, []adminapi.StrictMiddlewareFunc{ah.SessionMiddleware},
		adminapi.StrictHTTPServerOptions{RequestErrorHandlerFunc: adminapi.RequestErrorHandler, ResponseErrorHandlerFunc: adminapi.ErrorHandler})
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/", jsonNotFound)
	adminapi.HandlerWithOptions(adminStrict, adminapi.StdHTTPServerOptions{
		BaseURL: "/api/admin/v1", BaseRouter: apiMux, ErrorHandlerFunc: adminapi.RequestErrorHandler,
	})
	adminMux := http.NewServeMux()
	adminMux.Handle("/api/", apiCommon(csrf(o.Config.AllowedOrigins(), apiMux)))
	adminMux.HandleFunc("GET /bootstrap/{file}", s.serveBootstrap)
	adminMux.Handle("/", spa)
	s.admin = securityHeaders(onlyFrom(o.Config.TrustedProxyIP, adminMux))

	// Agent listener: /api/v1/* only.
	gh := &agentapi.Handlers{DB: o.DB, Grants: o.Grants, Agents: o.Agents}
	agentStrict := agentapi.NewStrictHandlerWithOptions(gh, []agentapi.StrictMiddlewareFunc{gh.AuthMiddleware},
		agentapi.StrictHTTPServerOptions{RequestErrorHandlerFunc: agentapi.RequestErrorHandler, ResponseErrorHandlerFunc: agentapi.ErrorHandler})
	agentMux := http.NewServeMux()
	agentMux.HandleFunc("/", jsonNotFound)
	agentapi.HandlerWithOptions(agentStrict, agentapi.StdHTTPServerOptions{
		BaseURL: "/api/v1", BaseRouter: agentMux, ErrorHandlerFunc: agentapi.RequestErrorHandler,
	})
	s.agent = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		apiCommon(agentMux).ServeHTTP(w, r)
	})
	return s, nil
}

// AdminHandler is the admin listener's handler.
func (s *Server) AdminHandler() http.Handler { return s.admin }

// AgentHandler is the agent listener's handler.
func (s *Server) AgentHandler() http.Handler { return s.agent }

// serveBootstrap serves /bootstrap/<name>.sh?t=<signed token>. The token
// replaces the session so curl can fetch it; it expires after 15 minutes.
func (s *Server) serveBootstrap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	name, ok := strings.CutSuffix(r.PathValue("file"), ".sh")
	fail := func() {
		http.Error(w, "echo 'lor: this bootstrap link is invalid or expired; get a fresh one from the host page' >&2; exit 1", http.StatusForbidden)
	}
	if !ok || !ca.HostNameRe.MatchString(name) || !s.opts.Hosts.VerifyBootstrapToken(name, r.URL.Query().Get("t")) {
		fail()
		return
	}
	host, err := s.opts.DB.GetHostByName(r.Context(), name)
	if err != nil {
		fail()
		return
	}
	script, err := s.opts.Hosts.RenderBootstrap(r.Context(), host)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	_ = s.opts.DB.InsertAudit(r.Context(), &store.AuditEvent{TS: s.opts.Grants.Now().Unix(), Actor: "bootstrap-link", Action: "host.bootstrap_fetched",
		HostID: host.ID, Details: map[string]any{"forwarded_for": r.Header.Get("X-Forwarded-For")}})
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Write(script)
}

func newHTTPServer(h http.Handler, writeTimeout time.Duration, log *slog.Logger) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
}

// Run serves both listeners until ctx ends. In dev mode the admin listener
// is plain HTTP (it only ever binds to 127.0.0.1).
func (s *Server) Run(ctx context.Context) error {
	cfg := s.opts.Config
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{s.opts.TLS}}

	adminLn, err := net.Listen("tcp", cfg.AdminListen)
	if err != nil {
		return fmt.Errorf("admin listener: %w", err)
	}
	proxy := cfg.TrustedProxyIP
	adminLn = filterListener{Listener: adminLn, allow: func(ip netip.Addr) bool { return ip == proxy }}
	if cfg.AdminTLS {
		adminLn = tls.NewListener(adminLn, tlsCfg)
	}
	agentLn, err := net.Listen("tcp", cfg.AgentListen)
	if err != nil {
		adminLn.Close()
		return fmt.Errorf("agent listener: %w", err)
	}
	agentLn = tls.NewListener(agentLn, tlsCfg)

	adminSrv := newHTTPServer(s.admin, 60*time.Second, s.log)
	agentSrv := newHTTPServer(s.agent, 90*time.Second, s.log) // long-poll waits up to 60s
	s.log.Info("admin listener", "addr", cfg.AdminListen, "tls", cfg.AdminTLS, "trusted_proxy", proxy.String())
	s.log.Info("agent listener", "addr", cfg.AgentListen, "tls_fingerprint", s.opts.TLSFingerprint)

	errc := make(chan error, 2)
	go func() { errc <- adminSrv.Serve(adminLn) }()
	go func() { errc <- agentSrv.Serve(agentLn) }()
	go s.opts.Grants.RunSweeper(ctx, s.opts.Admin.Sweep)

	select {
	case <-ctx.Done():
	case err = <-errc:
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	adminSrv.Shutdown(shutdownCtx)
	agentSrv.Shutdown(shutdownCtx)
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return err
}
