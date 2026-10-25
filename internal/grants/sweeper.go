package grants

import (
	"context"
	"time"

	"github.com/joshualent/lor/internal/store"
)

// SweepInterval is how often the sweeper runs.
const SweepInterval = 30 * time.Second

// Sweep moves expired, timed-out and moot items to their final states (with
// audit events) and deletes command-log entries older than the retention.
// Hosts enforce certificate expiry regardless.
func (s *Service) Sweep(ctx context.Context) error {
	open, err := s.db.ListGrants(ctx, store.GrantFilter{Statuses: []string{store.StatusPending, store.StatusOffered, store.StatusActive}})
	if err != nil {
		return err
	}
	for _, g := range open {
		if err := s.db.WithTx(ctx, func(tx *store.Tx) error {
			_, err := s.loadSettled(ctx, tx, g.ID)
			return err
		}); err != nil {
			return err
		}
	}
	// Pending extensions of grants that already ended.
	exts, err := s.db.ListExtensions(ctx, 0, store.ExtPending)
	if err != nil {
		return err
	}
	for _, e := range exts {
		if err := s.db.WithTx(ctx, func(tx *store.Tx) error {
			_, err := s.loadSettled(ctx, tx, e.GrantID)
			return err
		}); err != nil {
			return err
		}
	}
	cutoff := s.now().Add(-s.cfg.CommandLogRetention).Unix()
	_, err = s.db.DeleteCommandsBefore(ctx, cutoff)
	return err
}

// RunSweeper runs Sweep at start and every SweepInterval until ctx ends.
// extra, if non-nil, runs on the same schedule (session cleanup).
func (s *Service) RunSweeper(ctx context.Context, extra func(context.Context) error) {
	tick := time.NewTicker(SweepInterval)
	defer tick.Stop()
	for {
		if err := s.Sweep(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("sweeper", "err", err)
		}
		if extra != nil {
			if err := extra(ctx); err != nil && ctx.Err() == nil {
				s.log.Error("sweeper", "err", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
