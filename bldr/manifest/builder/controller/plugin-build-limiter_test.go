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
	// Exercise capacity-one plugin builds with deterministic goroutine scheduling.
	synctest.Test(t, func(t *testing.T) {
		// Bound the plugin acquisition goroutines to this test.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Hold the only plugin build permit with the first compiler.
		limiter := NewPluginBuildLimiter(1)
		first, err := limiter.Acquire(ctx, "bldr/plugin/compiler/go")
		if err != nil {
			t.Fatalf("acquire first plugin permit: %v", err)
		}

		// Start a second compiler that must wait for the first permit.
		attempted := make(chan struct{})
		started := make(chan error, 1)
		release := make(chan struct{})
		done := make(chan struct{})
		go func() {
			// Attempt the second plugin acquisition and publish its result.
			close(attempted)
			permit, err := limiter.Acquire(ctx, "bldr/plugin/compiler/js")
			started <- err
			if err != nil {
				return
			}

			// Release the second plugin permit when the test allows completion.
			select {
			case <-release:
				permit.Release()
				close(done)
			case <-ctx.Done():
			}
		}()

		// Confirm the second compiler stays blocked while the first permit is held.
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

		// Release the first compiler and await the second build completion.
		first.Release()
		if err := awaitPluginBuildSignal(t, started, "second plugin start"); err != nil {
			t.Fatalf("acquire second plugin permit: %v", err)
		}
		close(release)
		awaitPluginBuildSignal(t, done, "second plugin completion")
	})
}

func TestPluginBuildLimiterDoesNotBlockDependentManifestBuilder(t *testing.T) {
	// Cover parent and child Manifest builders that must share build capacity.
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

	// Check each dependent Manifest builder against a capacity-one limiter.
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// Bound the dependent Manifest acquisition to this test.
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				// Hold the parent Manifest permit while its child starts.
				limiter := NewPluginBuildLimiter(1)
				parent, err := limiter.Acquire(ctx, test.parentID)
				if err != nil {
					t.Fatalf("acquire parent Manifest permit: %v", err)
				}
				defer parent.Release()

				// Acquire and release the dependent child Manifest permit.
				childDone := make(chan error, 1)
				go func() {
					child, err := limiter.Acquire(ctx, test.childID)
					if err == nil {
						child.Release()
					}
					childDone <- err
				}()

				// Require the child Manifest to finish without waiting for its parent.
				if err := awaitPluginBuildSignal(t, childDone, "dependent child completion"); err != nil {
					t.Fatalf("acquire dependent child permit: %v", err)
				}
			})
		})
	}
}

func TestNewPluginBuildLimiterFromEnv(t *testing.T) {
	// Cover unset, unbounded, bounded, and invalid plugin capacity settings.
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

	// Verify the limiter behavior for each environment setting.
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Install the plugin concurrency setting for this test case.
			t.Setenv(PluginBuildConcurrencyEnv, test.value)
			if test.unset {
				if err := os.Unsetenv(PluginBuildConcurrencyEnv); err != nil {
					t.Fatalf("unset %s: %v", PluginBuildConcurrencyEnv, err)
				}
			}

			// Construct the limiter and verify environment validation.
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

			// Exercise the configured limiter under deterministic goroutine scheduling.
			synctest.Test(t, func(t *testing.T) {
				// Bound plugin permit acquisition to the scheduling test.
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				// Acquire the first plugin permit under the configured capacity.
				first, err := limiter.Acquire(ctx, "bldr/plugin/compiler/go")
				if err != nil {
					t.Fatalf("acquire first plugin permit: %v", err)
				}

				// Start a second plugin and record whether it can acquire a permit.
				attempted := make(chan struct{})
				started := make(chan error, 1)
				done := make(chan struct{})
				go func() {
					// Publish the second plugin acquisition result and release its permit.
					close(attempted)
					permit, err := limiter.Acquire(ctx, "bldr/plugin/compiler/js")
					started <- err
					if err != nil {
						return
					}
					permit.Release()
					close(done)
				}()

				// Observe whether the second plugin starts before the first permit releases.
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

				// Release the first permit and await completion of the second plugin.
				first.Release()
				if !startedBeforeRelease {
					if err := awaitPluginBuildSignal(t, started, "second plugin start"); err != nil {
						t.Fatalf("acquire second plugin permit: %v", err)
					}
				}
				awaitPluginBuildSignal(t, done, "second plugin completion")

				// Compare concurrent plugin starts with the configured capacity policy.
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
