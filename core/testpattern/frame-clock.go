package testpattern

import (
	"context"
	"io"
	"time"
)

// frameInterval sets the capture cadence of both test patterns.
const frameInterval = time.Second / 30

// frameClock paces one reader and releases it when the source closes.
type frameClock struct {
	// ticker sets the capture cadence and drops ticks the reader misses.
	ticker *time.Ticker
	// done closes when the source stops.
	done <-chan struct{}
	// cancel closes done, including when a read is waiting for a tick.
	cancel context.CancelFunc
}

// newFrameClock starts the clock for a test pattern source.
func newFrameClock() frameClock {
	// Give readers a shutdown signal independent of their own context.
	ctx, cancel := context.WithCancel(context.Background())
	return frameClock{
		ticker: time.NewTicker(frameInterval), //gostyle:timer deadline scheduled frame capture
		done:   ctx.Done(),
		cancel: cancel,
	}
}

// next waits for a capture tick, source shutdown, or reader cancellation.
func (c *frameClock) next(ctx context.Context) (time.Time, error) {
	select {
	case <-ctx.Done():
		return time.Time{}, ctx.Err()
	case <-c.done:
		return time.Time{}, io.EOF
	case captured := <-c.ticker.C:
		return captured, nil
	}
}

// Close stops the clock and releases the reader.
func (c *frameClock) Close() error {
	c.cancel()
	c.ticker.Stop()
	return nil
}
