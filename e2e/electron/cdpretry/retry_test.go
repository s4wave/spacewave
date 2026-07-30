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
