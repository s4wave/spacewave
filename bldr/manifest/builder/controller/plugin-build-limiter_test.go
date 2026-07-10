//go:build !js

package bldr_manifest_builder_controller

import (
	"context"
	"os"
	"testing"
	"testing/synctest"
	"time"
)

const pluginBuildLimiterWatchdogTimeout = 5 * time.Second

func TestPluginBuildLimiterCapacityOneSerializesPluginBuilds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		limiter := NewPluginBuildLimiter(1)
		first, err := limiter.Acquire(ctx, "bldr/plugin/compiler/go")
		if err != nil {
			t.Fatalf("acquire first plugin permit: %v", err)
		}

		attempted := make(chan struct{})
		started := make(chan error, 1)
		release := make(chan struct{})
		done := make(chan struct{})
		go func() {
			close(attempted)
			permit, err := limiter.Acquire(ctx, "bldr/plugin/compiler/js")
			started <- err
			if err != nil {
				return
			}
			select {
			case <-release:
				permit.Release()
				close(done)
			case <-ctx.Done():
			}
		}()

		awaitPluginBuildSignal(t, attempted, "second plugin acquisition attempt")
		synctest.Wait()
		select {
		case err := <-started:
			close(release)
			if err != nil {
				t.Fatalf("acquire second plugin permit: %v", err)
			}
			t.Fatal("second plugin started while the first permit was held")
		default:
		}

		first.Release()
		if err := awaitPluginBuildSignal(t, started, "second plugin start"); err != nil {
			t.Fatalf("acquire second plugin permit: %v", err)
		}
		close(release)
		awaitPluginBuildSignal(t, done, "second plugin completion")
	})
}

func TestPluginBuildLimiterDoesNotBlockDependentManifestBuilder(t *testing.T) {
	tests := []struct {
		name     string
		parentID string
		childID  string
	}{
		{
			name:     "dist parent waits for plugin child",
			parentID: "bldr/dist/compiler",
			childID:  "bldr/plugin/compiler/go",
		},
		{
			name:     "plugin parent waits for bundler child",
			parentID: "bldr/plugin/compiler/go",
			childID:  "bldr/web/bundler/vite/compiler",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				limiter := NewPluginBuildLimiter(1)
				parent, err := limiter.Acquire(ctx, test.parentID)
				if err != nil {
					t.Fatalf("acquire parent Manifest permit: %v", err)
				}
				defer parent.Release()

				childDone := make(chan error, 1)
				go func() {
					child, err := limiter.Acquire(ctx, test.childID)
					if err == nil {
						child.Release()
					}
					childDone <- err
				}()

				if err := awaitPluginBuildSignal(t, childDone, "dependent child completion"); err != nil {
					t.Fatalf("acquire dependent child permit: %v", err)
				}
			})
		})
	}
}

func TestNewPluginBuildLimiterFromEnv(t *testing.T) {
	tests := []struct {
		name           string
		value          string
		unset          bool
		wantError      bool
		wantConcurrent bool
	}{
		{name: "unset is unbounded", unset: true, wantConcurrent: true},
		{name: "empty is unbounded", wantConcurrent: true},
		{name: "zero is unbounded", value: "0", wantConcurrent: true},
		{name: "one bounds plugin builds", value: "1"},
		{name: "negative is rejected", value: "-1", wantError: true},
		{name: "malformed is rejected", value: "not-a-number", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(PluginBuildConcurrencyEnv, test.value)
			if test.unset {
				if err := os.Unsetenv(PluginBuildConcurrencyEnv); err != nil {
					t.Fatalf("unset %s: %v", PluginBuildConcurrencyEnv, err)
				}
			}

			limiter, err := NewPluginBuildLimiterFromEnv()
			if test.wantError {
				if err == nil {
					t.Fatalf("NewPluginBuildLimiterFromEnv() accepted %q", test.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewPluginBuildLimiterFromEnv(): %v", err)
			}

			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				first, err := limiter.Acquire(ctx, "bldr/plugin/compiler/go")
				if err != nil {
					t.Fatalf("acquire first plugin permit: %v", err)
				}

				attempted := make(chan struct{})
				started := make(chan error, 1)
				done := make(chan struct{})
				go func() {
					close(attempted)
					permit, err := limiter.Acquire(ctx, "bldr/plugin/compiler/js")
					started <- err
					if err != nil {
						return
					}
					permit.Release()
					close(done)
				}()

				awaitPluginBuildSignal(t, attempted, "second plugin acquisition attempt")
				synctest.Wait()
				startedBeforeRelease := false
				select {
				case err := <-started:
					if err != nil {
						t.Fatalf("acquire second plugin permit: %v", err)
					}
					startedBeforeRelease = true
				default:
				}

				first.Release()
				if !startedBeforeRelease {
					if err := awaitPluginBuildSignal(t, started, "second plugin start"); err != nil {
						t.Fatalf("acquire second plugin permit: %v", err)
					}
				}
				awaitPluginBuildSignal(t, done, "second plugin completion")

				if startedBeforeRelease != test.wantConcurrent {
					if test.wantConcurrent {
						t.Fatal("second plugin was blocked by an unbounded limiter")
					}
					t.Fatal("second plugin started before the capacity-one permit released")
				}
			})
		})
	}
}

func awaitPluginBuildSignal[T any](
	t *testing.T,
	signal <-chan T,
	description string,
) T {
	t.Helper()
	watchdog := time.NewTimer(pluginBuildLimiterWatchdogTimeout)
	defer watchdog.Stop()

	select {
	case value := <-signal:
		return value
	case <-watchdog.C:
		t.Fatalf("timed out waiting for %s", description)
		var zero T
		return zero
	}
}
