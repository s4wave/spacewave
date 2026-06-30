package bldr_dist_compiler

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/sirupsen/logrus"
)

func TestWaitForEmbedManifestValueErrorsWhenIdleWithoutValue(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
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
		t.Fatal("expected idle FetchManifest error")
	}
	if ctx.Err() != nil {
		t.Fatalf("wait reached context deadline instead of idle error: %v", err)
	}
	if !strings.Contains(err.Error(), "FetchManifest became idle without a manifest value") {
		t.Fatalf("error = %q, want idle without manifest value", err.Error())
	}
}
