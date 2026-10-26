// Command lor-server is Lor's control plane: the SSH CA, the agent API and
// the admin web UI (plan sections 3 and 15).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/joshualent/lor/internal/auth"
	"github.com/joshualent/lor/internal/ca"
	"github.com/joshualent/lor/internal/config"
	"github.com/joshualent/lor/internal/grants"
	"github.com/joshualent/lor/internal/hosts"
	"github.com/joshualent/lor/internal/server"
	"github.com/joshualent/lor/internal/store"
	"github.com/joshualent/lor/internal/tlscert"
	"github.com/joshualent/lor/internal/version"
	"github.com/joshualent/lor/web"
)

const defaultConfig = "/etc/lor/lor-server.toml"

func main() {
	if err := newRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "lor-server:", err)
		os.Exit(1)
	}
}

type devFlags struct {
	enabled     bool
	adminListen string
	agentListen string
	dataDir     string
	extraOrigin []string
}

func newRoot() *cobra.Command {
	var cfgPath string
	var dev devFlags
	root := &cobra.Command{
		Use:           "lor-server",
		Short:         "Lor: temporary, approved SSH access for AI agents",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Version,
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", defaultConfig, "path to lor-server.toml")

	serve := &cobra.Command{
		Use:   "serve",
		Short: "Run the server (admin and agent listeners)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd.Context(), cfgPath, dev)
		},
	}
	f := serve.Flags()
	f.BoolVar(&dev.enabled, "dev", false, "development mode: plain-HTTP admin listener on 127.0.0.1, RP ID localhost, throwaway data dir")
	f.StringVar(&dev.adminListen, "dev-admin-listen", "127.0.0.1:8444", "dev: admin listener address (127.0.0.1 only)")
	f.StringVar(&dev.agentListen, "dev-agent-listen", "127.0.0.1:8443", "dev: agent listener address")
	f.StringVar(&dev.dataDir, "dev-data-dir", "", "dev: data dir to use instead of a temporary one")
	f.StringSliceVar(&dev.extraOrigin, "dev-origin", []string{"http://localhost:5173"}, "dev: extra allowed origins (Vite dev server)")
	root.AddCommand(serve)

	admin := &cobra.Command{Use: "admin", Short: "Admin recovery commands (run on the Lor LXC console)"}
	admin.AddCommand(&cobra.Command{
		Use:   "setup-code",
		Short: "Print a one-time code (valid 15 minutes) to register the first passkey at /setup",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			db, err := store.Open(cmd.Context(), filepath.Join(cfg.DataDir, "lor.db"))
			if err != nil {
				return err
			}
			defer db.Close()
			code, exp, err := auth.CreateSetupCode(cmd.Context(), db, time.Now())
			if err != nil {
				return err
			}
			fmt.Printf("Setup code: %s\nValid until %s. Open %s/setup and enter it.\n", code, exp.Format("15:04"), cfg.PublicURL)
			return nil
		},
	})
	admin.AddCommand(&cobra.Command{
		Use:   "reset",
		Short: "Delete all passkeys and sessions (then run setup-code again)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			db, err := store.Open(cmd.Context(), filepath.Join(cfg.DataDir, "lor.db"))
			if err != nil {
				return err
			}
			defer db.Close()
			if err := auth.Reset(cmd.Context(), db, time.Now()); err != nil {
				return err
			}
			fmt.Println("All passkeys and sessions were deleted. Run `lor-server admin setup-code` to register a new passkey.")
			return nil
		},
	})
	root.AddCommand(admin)
	return root
}

func devConfig(d devFlags) (config.Config, func(), error) {
	cleanup := func() {}
	host, port, err := net.SplitHostPort(d.adminListen)
	if err != nil {
		return config.Config{}, cleanup, err
	}
	if host != "127.0.0.1" {
		return config.Config{}, cleanup, errors.New("--dev only binds the admin listener to 127.0.0.1")
	}
	dir := d.dataDir
	if dir == "" {
		dir, err = os.MkdirTemp("", "lor-dev-*")
		if err != nil {
			return config.Config{}, cleanup, err
		}
		cleanup = func() { os.RemoveAll(dir) }
	}
	cfg := config.Default()
	cfg.Dev = true
	cfg.AdminTLS = false
	cfg.DataDir = dir
	cfg.PublicURL = "http://localhost:" + port
	cfg.AdminListen = d.adminListen
	cfg.AgentListen = d.agentListen
	cfg.TrustedProxy = "127.0.0.1"
	cfg.ExtraOrigins = d.extraOrigin
	return cfg, cleanup, cfg.Finish()
}

func runServe(parent context.Context, cfgPath string, dev devFlags) error {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	var cfg config.Config
	var err error
	if dev.enabled {
		var cleanup func()
		cfg, cleanup, err = devConfig(dev)
		defer cleanup()
		if err != nil {
			return err
		}
		log.Warn("DEVELOPMENT MODE: plain-HTTP admin listener, RP ID localhost", "data_dir", cfg.DataDir)
	} else {
		cfg, err = config.Load(cfgPath)
		if err != nil {
			return err
		}
	}

	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	db, err := store.Open(ctx, filepath.Join(cfg.DataDir, "lor.db"))
	if err != nil {
		return err
	}
	defer db.Close()

	authority, created, err := ca.LoadOrGenerate(filepath.Join(cfg.DataDir, "ca"), ca.WithAbsoluteMax(cfg.AbsoluteMaxDuration.D()))
	if err != nil {
		return err
	}
	if created {
		log.Warn("generated a new CA key; back up <data_dir>/ca off the box", "fingerprint", authority.Fingerprint())
		_ = db.InsertAudit(ctx, &store.AuditEvent{TS: time.Now().Unix(), Actor: "system", Action: "ca.generated",
			Details: map[string]any{"fingerprint": authority.Fingerprint()}})
	}
	log.Info("CA", "fingerprint", authority.Fingerprint())

	adminHost, _, _ := net.SplitHostPort(cfg.AdminListen)
	agentHost, _, _ := net.SplitHostPort(cfg.AgentListen)
	sans := append([]string{adminHost, agentHost}, cfg.TLSExtraSANs...)
	cert, fp, tlsCreated, err := tlscert.LoadOrGenerate(filepath.Join(cfg.DataDir, "tls"), sans)
	if err != nil {
		return err
	}
	if tlsCreated {
		log.Warn("generated a new self-signed TLS certificate; copy it to Caddy", "path", filepath.Join(cfg.DataDir, "tls", tlscert.CertFile), "fingerprint", fp)
		_ = db.InsertAudit(ctx, &store.AuditEvent{TS: time.Now().Unix(), Actor: "system", Action: "tls.generated",
			Details: map[string]any{"fingerprint": fp}})
	}
	if missing := tlscert.MissingSANs(cert, []string{adminHost, agentHost}); len(missing) > 0 {
		log.Warn("the TLS certificate doesn't cover these listen IPs; delete <data_dir>/tls to regenerate", "missing", missing)
	}

	grantSvc := grants.New(db, authority, grants.Config{
		PublicURL:           cfg.PublicURL,
		DefaultDuration:     cfg.DefaultGrantDuration.D(),
		AgentRequestMax:     cfg.AgentRequestMax.D(),
		AbsoluteMax:         cfg.AbsoluteMaxDuration.D(),
		ApprovalTimeout:     cfg.RequestApprovalTimeout.D(),
		CommandLogRetention: cfg.CommandLogRetention.D(),
	}, nil)
	hostSvc, err := hosts.New(ctx, db, authority, cfg.PublicURL, nil, nil)
	if err != nil {
		return err
	}
	adminAuth, err := auth.NewAdmin(ctx, db, cfg.RPID, cfg.AllowedOrigins(), nil)
	if err != nil {
		return err
	}
	srv, err := server.New(server.Options{
		Config: &cfg, DB: db, CA: authority, Grants: grantSvc, Hosts: hostSvc, Admin: adminAuth,
		Agents: auth.NewAgents(db, nil), TLS: cert, TLSFingerprint: fp, Web: web.Files(), Logger: log,
	})
	if err != nil {
		return err
	}

	if cfg.Dev {
		if required, _ := adminAuth.SetupRequired(ctx); required {
			code, _, err := auth.CreateSetupCode(ctx, db, time.Now())
			if err != nil {
				return err
			}
			fmt.Printf("LOR_DEV_SETUP_CODE=%s\n", code)
		}
		fmt.Printf("LOR_DEV_URL=%s\nLOR_DEV_AGENT_URL=https://%s\nLOR_DEV_TLS_FINGERPRINT=%s\n", cfg.PublicURL, cfg.AgentListen, fp)
	}
	return srv.Run(ctx)
}
