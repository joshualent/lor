package grants

import (
	"context"
	"sync"
	"time"
)

// Notifier wakes long-polling callers when a grant changes.
type Notifier struct {
	mu      sync.Mutex
	waiters map[int64]map[chan struct{}]struct{}
	all     map[chan struct{}]struct{}
}

// NewNotifier returns an empty Notifier.
func NewNotifier() *Notifier {
	return &Notifier{waiters: map[int64]map[chan struct{}]struct{}{}, all: map[chan struct{}]struct{}{}}
}

// Subscribe registers interest in grant id (0 = any grant). Call the
// returned cancel function when done. Subscribe before reading state so no
// change can be missed between the read and the wait.
func (n *Notifier) Subscribe(id int64) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	n.mu.Lock()
	if id == 0 {
		n.all[ch] = struct{}{}
	} else {
		if n.waiters[id] == nil {
			n.waiters[id] = map[chan struct{}]struct{}{}
		}
		n.waiters[id][ch] = struct{}{}
	}
	n.mu.Unlock()
	return ch, func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		if id == 0 {
			delete(n.all, ch)
			return
		}
		delete(n.waiters[id], ch)
		if len(n.waiters[id]) == 0 {
			delete(n.waiters, id)
		}
	}
}

// Notify wakes everyone waiting on grant id.
func (n *Notifier) Notify(id int64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for ch := range n.waiters[id] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	for ch := range n.all {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Wait blocks until ch fires, the timeout passes or ctx ends.
func Wait(ctx context.Context, ch <-chan struct{}, timeout time.Duration) {
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-ch:
	case <-t.C:
	case <-ctx.Done():
	}
}
