//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/util/gitroot"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// TestStartupBuildCacheInvalidatesOnSourceChange boots the harness twice
// against the preserved startup build cache and mutates a bundled app source
// between boots. The second boot must reject the cached spacewave-app Manifest
// and rebuild it: an unchanged settled digest means startup reused a stale
// build after the source changed.
func TestStartupBuildCacheInvalidatesOnSourceChange(t *testing.T) {
	// Enable the startup build cache.
	t.Setenv("E2E_WASM_STARTUP_BUILD_CACHE", "true")

	// Snapshot app source and restore it when the test ends.
	repoRoot, err := gitroot.FindRepoRoot()
	if err != nil {
		t.Fatal(errors.Wrap(err, "find repo root"))
	}
	appSource := filepath.Join(repoRoot, "app", "App.tsx")
	original, err := os.ReadFile(appSource)
	if err != nil {
		t.Fatal(errors.Wrap(err, "read app source"))
	}
	t.Cleanup(func() {
		if err := os.WriteFile(appSource, original, 0o644); err != nil {
			t.Errorf("restore app source: %v", err)
		}
	})

	// Bound the two boots.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Boot once and record the startup manifest digests.
	first, err := Boot(ctx, le,
		WithManifestBuildTimeout(20*time.Minute),
	)
	if err != nil {
		t.Fatalf("first boot: %v", err)
	}
	firstDigests, err := first.preflightStartupManifests(ctx)
	if err != nil {
		first.Release()
		t.Fatalf("settle first boot manifests: %v", err)
	}
	first.Release()

	// Append a probe comment to the app source.
	mutated := append([]byte(nil), original...)
	mutated = append(mutated, []byte("\n// e2e startup cache invalidation probe\n")...)
	if err := os.WriteFile(appSource, mutated, 0o644); err != nil {
		t.Fatal(errors.Wrap(err, "mutate app source"))
	}

	// Boot again and record the new startup manifest digests.
	second, err := Boot(ctx, le,
		WithManifestBuildTimeout(20*time.Minute),
	)
	if err != nil {
		t.Fatalf("second boot: %v", err)
	}
	defer second.Release()
	secondDigests, err := second.preflightStartupManifests(ctx)
	if err != nil {
		t.Fatalf("settle second boot manifests: %v", err)
	}

	// Require the app manifest digest to change.
	const pluginID = "spacewave-app"
	firstDigest, ok := firstDigests[pluginID]
	if !ok {
		t.Fatalf("plugin %q missing from first boot digests: %v", pluginID, firstDigests)
	}
	secondDigest := secondDigests[pluginID]
	if secondDigest == firstDigest {
		t.Fatalf(
			"startup build cache reused stale %q manifest after source change: digest %s",
			pluginID,
			firstDigest,
		)
	}
}
