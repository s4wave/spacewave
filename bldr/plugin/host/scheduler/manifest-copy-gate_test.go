package plugin_host_scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/bucket"
)

type testManifestCopyGate struct {
	readyCtr    *ccontainer.CContainer[bool]
	waitOnce    sync.Once
	waitStarted chan struct{}
	waitCount   atomic.Int32
}

func newTestManifestCopyGate(ready bool) *testManifestCopyGate {
	return &testManifestCopyGate{
		readyCtr:    ccontainer.NewCContainer(ready),
		waitStarted: make(chan struct{}),
	}
}

func (g *testManifestCopyGate) IsReady() bool {
	return g.readyCtr.GetValue()
}

func (g *testManifestCopyGate) WaitReady(ctx context.Context) error {
	g.waitCount.Add(1)
	g.waitOnce.Do(func() { close(g.waitStarted) })
	_, err := g.readyCtr.WaitValue(ctx, nil)
	return err
}

func TestManifestCopyGateDefersOnlyPreReadyDynamicCopies(t *testing.T) {
	gate := newTestManifestCopyGate(false)
	controller := &Controller{
		conf:                &Config{NoCopyBucketIds: []string{"dist/project"}},
		manifestCopyGateCtr: ccontainer.NewCContainer[ManifestCopyGate](gate),
	}
	instance := &pluginInstance{c: controller}
	suppressed := &bldr_manifest.ManifestSnapshot{
		ManifestRef: &bucket.ObjectRef{BucketId: "dist/project"},
	}
	dynamic := &bldr_manifest.ManifestSnapshot{
		ManifestRef: &bucket.ObjectRef{BucketId: "dynamic-provider"},
	}

	if class := instance.classifyManifestCopy(suppressed); class != manifestCopyClassSuppressed {
		t.Fatalf("suppressed copy class = %q, want %q", class, manifestCopyClassSuppressed)
	}
	if class := instance.classifyManifestCopy(dynamic); class != manifestCopyClassAfterStartupGroupReady {
		t.Fatalf("pre-ready dynamic copy class = %q, want %q", class, manifestCopyClassAfterStartupGroupReady)
	}

	waitErrCh := make(chan error, 1)
	go func() {
		waitErrCh <- instance.waitForManifestCopyReady(
			t.Context(),
			manifestCopyClassAfterStartupGroupReady,
			dynamic,
			nil,
		)
	}()
	<-gate.waitStarted
	select {
	case err := <-waitErrCh:
		t.Fatalf("dynamic copy gate returned before readiness: %v", err)
	default:
	}

	gate.readyCtr.SetValue(true)
	if err := <-waitErrCh; err != nil {
		t.Fatal(err)
	}
	if class := instance.classifyManifestCopy(dynamic); class != manifestCopyClassImmediate {
		t.Fatalf("post-startup dynamic copy class = %q, want %q", class, manifestCopyClassImmediate)
	}
	if got := gate.waitCount.Load(); got != 1 {
		t.Fatalf("gate wait count = %d, want 1", got)
	}
}
