//go:build !js

package bldr_manifest_builder_controller

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/sync/semaphore"
)

const (
	// PluginBuildConcurrencyEnv configures the maximum concurrent plugin builds.
	PluginBuildConcurrencyEnv    = "BLDR_PLUGIN_BUILD_CONCURRENCY"
	pluginCompilerConfigIDPrefix = "bldr/plugin/compiler/"
)

// PluginBuildLimiter bounds concurrent whole-plugin builds within one builder
// controller factory. Plugin builds hold a slot while awaiting subordinate
// bundler manifests; only plugin compiler configs acquire slots, so those
// dependencies never contend for the same capacity.
type PluginBuildLimiter struct {
	semaphore *semaphore.Weighted
}

// NewPluginBuildLimiter constructs a plugin build limiter. A non-positive
// concurrency leaves plugin builds unbounded.
func NewPluginBuildLimiter(concurrency int64) *PluginBuildLimiter {
	limiter := &PluginBuildLimiter{}
	if concurrency > 0 {
		limiter.semaphore = semaphore.NewWeighted(concurrency)
	}
	return limiter
}

// NewPluginBuildLimiterFromEnv constructs a plugin build limiter from
// PluginBuildConcurrencyEnv. An unset, empty, or zero value leaves builds
// unbounded.
func NewPluginBuildLimiterFromEnv() (*PluginBuildLimiter, error) {
	raw := strings.TrimSpace(os.Getenv(PluginBuildConcurrencyEnv))
	if raw == "" {
		return NewPluginBuildLimiter(0), nil
	}
	concurrency, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "parse %s", PluginBuildConcurrencyEnv)
	}
	if concurrency < 0 {
		return nil, errors.Errorf("%s must not be negative", PluginBuildConcurrencyEnv)
	}
	return NewPluginBuildLimiter(concurrency), nil
}

// Acquire waits for one plugin build slot when controllerConfigID identifies a
// plugin compiler. Other Manifest builders remain outside the limiter so they
// can satisfy dependencies of a plugin that holds a slot.
func (l *PluginBuildLimiter) Acquire(
	ctx context.Context,
	controllerConfigID string,
) (PluginBuildPermit, error) {
	if l.semaphore == nil || !strings.HasPrefix(controllerConfigID, pluginCompilerConfigIDPrefix) {
		return PluginBuildPermit{}, nil
	}
	if err := l.semaphore.Acquire(ctx, 1); err != nil {
		return PluginBuildPermit{}, err
	}
	return PluginBuildPermit{limiter: l}, nil
}
