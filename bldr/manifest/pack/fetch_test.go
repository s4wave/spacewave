package bldr_manifest_pack

import (
	"context"
	"io"
	"testing"
	"time"

	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/sirupsen/logrus"
)

// TestResolveManifestTupleWaitsWhenIdleWithoutValue preserves late-producer readiness.
func TestResolveManifestTupleWaitsWhenIdleWithoutValue(t *testing.T) {
	// A bus without manifest producers must remain pending until cancellation.
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	t.Cleanup(cancel)
	log := logrus.New()
	log.SetOutput(io.Discard)
	b, _, err := controllerbus_core.NewCoreBus(ctx, logrus.NewEntry(log))
	if err != nil {
		t.Fatal(err)
	}

	// Resolve an absent tuple and distinguish idle state from terminal failure.
	ref, err := ResolveManifestTuple(
		ctx,
		b,
		&ManifestTuple{ManifestId: "missing-core", PlatformId: "web/js/wasm", ObjectKey: "dist/missing-core"},
		bldr_manifest.BuildType_RELEASE.String(),
	)
	if ref != nil {
		t.Fatal("unexpected manifest while no producer has a value")
	}
	if err == nil {
		t.Fatal("expected context timeout while waiting for FetchManifest")
	}
	if ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("wait returned before context deadline: err=%v ctx=%v", err, ctx.Err())
	}
}
