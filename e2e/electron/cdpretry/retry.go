//go:build !skip_e2e && !js

// Package cdpretry contains the Electron renderer CDP retry policy.
package cdpretry

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

const (
	waitForPagePollInterval = 100 * time.Millisecond
	evaluateRetryBudget     = 5 * time.Second
	evaluateRetryAttempts   = int(evaluateRetryBudget / waitForPagePollInterval)
	evaluateRetryDelay      = waitForPagePollInterval
)

// Page evaluates JavaScript in a renderer page.
type Page interface {
	Evaluate(expression string, arg ...any) (any, error)
}

// WaitForPage returns the next renderer page available through CDP.
type WaitForPage func(context.Context) (Page, error)

// EvaluateUserAgent evaluates navigator.userAgent, retrying while the renderer
// replaces its execution context during startup.
func EvaluateUserAgent(ctx context.Context, page Page, waitForPage WaitForPage) (string, error) {
	return evaluateUserAgent(ctx, page, waitForPage, evaluateRetryAttempts, evaluateRetryDelay)
}

func evaluateUserAgent(
	ctx context.Context,
	page Page,
	waitForPage WaitForPage,
	maxAttempts int,
	delay time.Duration,
) (string, error) {
	if maxAttempts <= 0 {
		return "", errors.Errorf("evaluate navigator.userAgent requires a positive attempt count, got %d", maxAttempts)
	}

	var lastEvaluateErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		raw, err := page.Evaluate(`() => navigator.userAgent`)
		if err == nil {
			ua, ok := raw.(string)
			if !ok {
				return "", errors.Errorf("expected string user agent, got %T", raw)
			}
			return ua, nil
		}
		lastEvaluateErr = err

		if attempt == maxAttempts {
			return "", evaluateRetryError(lastEvaluateErr, attempt, ctx.Err())
		}

		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return "", evaluateRetryError(lastEvaluateErr, attempt, ctx.Err())
			case <-timer.C:
			}
		}

		page, err = waitForPage(ctx)
		if err != nil {
			return "", evaluateRetryError(lastEvaluateErr, attempt, err)
		}
	}
	return "", errors.Errorf("evaluate navigator.userAgent did not run")
}

func evaluateRetryError(lastEvaluateErr error, attempts int, secondary error) error {
	if secondary != nil {
		return errors.Wrapf(
			lastEvaluateErr,
			"evaluate navigator.userAgent failed after %d attempts; secondary error: %v",
			attempts,
			secondary,
		)
	}
	return errors.Wrapf(lastEvaluateErr, "evaluate navigator.userAgent failed after %d attempts", attempts)
}
