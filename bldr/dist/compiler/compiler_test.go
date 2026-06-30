package bldr_dist_compiler

import (
	"context"
	"io"
	"testing"
	"time"

	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/sirupsen/logrus"
)

func TestWaitForEmbedManifestValueWaitsWhenIdleWithoutValue(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	log := logrus.New()
	log.SetOutput(io.Discard)
	b, _, err := controllerbus_core.NewCoreBus(ctx, logrus.NewEntry(log))
	if err != nil {
		t.Fatal(err)
	}

	ref, err := waitForEmbedManifestValue(
		ctx,
		b,
		&EmbedManifest{ManifestId: "missing-core", PlatformId: "web/js/wasm"},
		bldr_manifest.BuildType_RELEASE,
	)
	if ref != nil {
		defer ref.Release()
	}
	if err == nil {
		t.Fatal("expected context timeout while waiting for FetchManifest")
	}
	if ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("wait returned before context deadline: err=%v ctx=%v", err, ctx.Err())
	}
}
