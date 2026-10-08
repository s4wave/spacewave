//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	cdc "github.com/aperturerobotics/controllerbus/directive/controller"
	packedmsg "github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/core/cdn"
	cdn_world_controller "github.com/s4wave/spacewave/core/cdn/world/controller"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// TestReleaseMetadataRoutineWakesMountOnEmptySpace proves a release World
// mounted on a Space with no published head mounts its engine once a head is
// published and the launcher's routine restarts, with no caller sending the
// refresh by hand.
func TestReleaseMetadataRoutineWakesMountOnEmptySpace(t *testing.T) {
	// Bound the test.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// Serve no head until published. The mount's own fetch, the engine
	// build's fetch, and the launcher's refresh are the first three requests.
	const spaceID = "release-space"
	pointer := encodeTestRootPointer(t, spaceID)
	var published atomic.Bool
	var requests atomic.Int32
	launcherAsked := make(chan struct{})
	cdnServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 3 {
			close(launcherAsked)
		}
		if !published.Load() {
			http.Error(w, "no published head", http.StatusNotFound)
			return
		}
		_, _ = w.Write(pointer)
	}))
	defer cdnServer.Close()

	// Mount the release World on the bus.
	le := logrus.NewEntry(logrus.New())
	b := inmem.NewBus(cdc.NewController(ctx, le))
	mount := cdn_world_controller.NewController(le, b, cdn_world_controller.NewConfig(releaseWorldEngineID, spaceID, cdnServer.URL))
	relMount, err := b.AddController(ctx, mount, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relMount()

	// Start the launcher's routine; it asks the empty mount to fetch.
	ctrl := newReleaseMetadataRoutineTestController(le, b, t.TempDir())
	ctrl.releaseMetadataRoutine.SetContext(ctx, true)
	defer ctrl.releaseMetadataRoutine.ClearContext()
	select {
	case <-launcherAsked:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// Publish a head, then announce it by restarting the routine.
	published.Store(true)
	ctrl.RecheckReleaseMetadata()

	// The mount publishes its engine.
	eng, _, ref, err := world.ExLookupWorldEngine(ctx, b, false, releaseWorldEngineID, nil)
	if err != nil || eng == nil {
		t.Fatalf("release world did not mount after the head was published: %v", err)
	}
	ref.Release()
}

// encodeTestRootPointer returns the root pointer of spaceID whose plain World
// has an empty head.
func encodeTestRootPointer(t *testing.T, spaceID string) []byte {
	t.Helper()
	state, err := (&sobject_world_engine.InnerState{HeadRef: &bucket.ObjectRef{}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	inner, err := (&sobject.SOCheckpointInner{
		SharedObjectId: spaceID,
		ConfigHash:     make([]byte, sha256.Size),
		ReplayVersion:  sobject.SOReplayVersion,
		StateData:      state,
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	data, err := (&cdn.CdnRootPointer{
		SpaceId:    spaceID,
		Checkpoint: &sobject.SOCheckpoint{Inner: inner},
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return []byte(packedmsg.EncodePackedMessage(data))
}
