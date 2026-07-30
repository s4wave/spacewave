//go:build !skip_e2e && !js

package cdpretry

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
)

type failingPage struct {
	err error
}

func (p failingPage) Evaluate(string, ...any) (any, error) {
	return nil, p.err
}

func TestEvaluateUserAgentReportsLastEvaluateError(t *testing.T) {
	evaluateErr := errors.New("CDP execution context was destroyed")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := evaluateUserAgent(
		ctx,
		failingPage{err: evaluateErr},
		func(ctx context.Context) (Page, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				return failingPage{err: evaluateErr}, nil
			}
		},
		3,
		0,
	)
	if err == nil {
		t.Fatal("expected navigator.userAgent evaluation error")
	}
	if !strings.Contains(err.Error(), evaluateErr.Error()) {
		t.Fatalf("expected evaluation error %q in %q", evaluateErr, err)
	}
	if !strings.Contains(err.Error(), "after 3 attempts") {
		t.Fatalf("expected attempt count in %q", err)
	}
}

func TestEvaluateUserAgentReportsContextAfterEvaluateError(t *testing.T) {
	evaluateErr := errors.New("CDP execution context was destroyed")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := evaluateUserAgent(
		ctx,
		failingPage{err: evaluateErr},
		func(ctx context.Context) (Page, error) {
			return nil, ctx.Err()
		},
		3,
		0,
	)
	if err == nil {
		t.Fatal("expected navigator.userAgent evaluation error")
	}
	if !strings.Contains(err.Error(), evaluateErr.Error()) {
		t.Fatalf("expected evaluation error %q in %q", evaluateErr, err)
	}
	if !strings.Contains(err.Error(), "secondary error: context canceled") {
		t.Fatalf("expected context error in %q", err)
	}
}

// stringPage answers with a fixed user agent.
type stringPage struct {
	userAgent string
}

func (p stringPage) Evaluate(string, ...any) (any, error) {
	return p.userAgent, nil
}

// TestEvaluateUserAgentSucceedsOnTheReplacementPage covers the case the retry
// exists for. The renderer destroys its execution context during startup, so
// the page handle the caller holds is dead and only a fresh one answers.
func TestEvaluateUserAgentSucceedsOnTheReplacementPage(t *testing.T) {
	evaluateErr := errors.New("CDP execution context was destroyed")
	waits := 0

	ua, err := evaluateUserAgent(
		context.Background(),
		failingPage{err: evaluateErr},
		func(context.Context) (Page, error) {
			waits++
			return stringPage{userAgent: "Electron/38.0.0"}, nil
		},
		3,
		0,
	)
	if err != nil {
		t.Fatalf("expected the replacement page to answer: %v", err)
	}
	if ua != "Electron/38.0.0" {
		t.Fatalf("got user agent %q, want the replacement page's", ua)
	}
	if waits != 1 {
		t.Fatalf("fetched a replacement page %d times, want 1", waits)
	}
}

// TestEvaluateUserAgentReportsReplacementFailure pins what happens when the
// renderer never offers a usable page.
func TestEvaluateUserAgentReportsReplacementFailure(t *testing.T) {
	evaluateErr := errors.New("CDP execution context was destroyed")
	waitErr := errors.New("no renderer page appeared")

	_, err := evaluateUserAgent(
		context.Background(),
		failingPage{err: evaluateErr},
		func(context.Context) (Page, error) {
			return nil, waitErr
		},
		3,
		0,
	)
	if err == nil {
		t.Fatal("expected an error when no replacement page appears")
	}
	if !strings.Contains(err.Error(), evaluateErr.Error()) {
		t.Fatalf("expected evaluation error %q in %q", evaluateErr, err)
	}
	if !strings.Contains(err.Error(), waitErr.Error()) {
		t.Fatalf("expected replacement error %q in %q", waitErr, err)
	}
}
