package internal

import (
	"context"
	"sync"
	"time"
)

// limiter spaces requests at least interval apart. MusicBrainz allows an
// average of one request per second per client.
type limiter struct {
	next     time.Time
	interval time.Duration
	mu       sync.Mutex
}

func newLimiter(interval time.Duration) *limiter {
	return &limiter{interval: interval}
}

// wait reserves the next slot and blocks until it arrives or ctx is done.
func (l *limiter) wait(ctx context.Context) error {
	if l == nil || l.interval <= 0 {
		return ctx.Err()
	}
	l.mu.Lock()
	now := time.Now()
	at := l.next
	if at.Before(now) {
		at = now
	}
	l.next = at.Add(l.interval)
	l.mu.Unlock()

	d := at.Sub(now)
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
