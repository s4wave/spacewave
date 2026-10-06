package manifest_fetch_world

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestFetchManifestIdleOnlyAfterReadingMountedWorld checks that a resolver
// whose World engine is already mounted reports idle only after it has read
// the World, so a caller waiting for idle sees the stored manifest.
func TestFetchManifestIdleOnlyAfterReadingMountedWorld(t *testing.T) {
	// Mount a World engine with a manifest store.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	ws := world.NewEngineWorldState(tb.Engine, true)
	const storeKey = "release/manifests"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err)
	}

	// Store one manifest, writing its block through the World.
	meta := &manifest.ManifestMeta{ManifestId: "core", BuildType: "production", PlatformId: "js", Rev: 1}
	stage, err := ws.StageWorldState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer stage.Release()
	ref, err := world.AccessObject(ctx, stage.AccessWorldState, nil, func(cursor *block.Cursor) error {
		cursor.SetBlock(manifest.NewManifest(meta, "entrypoint"), true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	key := manifest.NewManifestKey(storeKey, meta)
	if err := bldr_manifest_world.ExStoreManifestOp(ctx, ws, peer.ID("test"), key, []string{storeKey}, manifest.NewManifestRef(meta, ref)); err != nil {
		t.Fatal(err)
	}

	// Resolve FetchManifest, recording the value count at the first idle.
	ctrl := NewController(logrus.NewEntry(logrus.New()), tb.Bus, &Config{
		EngineId:     tb.EngineID,
		ObjectKeys:   []string{storeKey},
		DisableWatch: true,
	})
	defer func() { _ = ctrl.Close() }()
	handler := &idleTestHandler{idle: make(chan int, 1)}
	resolver := &fetchManifestResolver{c: ctrl, dir: manifest.NewFetchManifest("core", nil, []string{"js"}, 0)}
	go func() { _ = resolver.Resolve(ctx, handler) }()

	// The first idle must follow the stored manifest's value.
	select {
	case values := <-handler.idle:
		if values != 1 {
			t.Fatalf("FetchManifest went idle with %d values, want 1", values)
		}
	case <-ctx.Done():
		t.Fatal("FetchManifest did not go idle")
	}
}

// idleTestHandler reports the value count when the resolver first goes idle.
type idleTestHandler struct {
	// mtx guards the embedded handler, which the bus calls concurrently.
	mtx sync.Mutex
	collectionTestHandler
	// idle receives the value count at the first idle.
	idle chan int
	// reported is set after the first idle is sent.
	reported bool
}

// AddValue records the value.
func (h *idleTestHandler) AddValue(value directive.Value) (uint32, bool) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return h.collectionTestHandler.AddValue(value)
}

// ClearValues drops every value.
func (h *idleTestHandler) ClearValues() []uint32 {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return h.collectionTestHandler.ClearValues()
}

// MarkIdle reports the value count on the first idle.
func (h *idleTestHandler) MarkIdle(idle bool) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	if idle && !h.reported {
		h.reported = true
		h.idle <- len(h.values)
	}
}
