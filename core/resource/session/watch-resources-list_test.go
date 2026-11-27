package resource_session_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/space"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_canvas_world "github.com/s4wave/spacewave/sdk/canvas/world"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

var errResourcesListProjectionCaptured = errors.New("resources list projection captured")

// TestWatchResourcesListProjectsDurableSpaceIndexObjectTypes verifies each
// listed Space carries the ObjectType selected inside its own durable world,
// while a Space without an index object remains generic.
func TestWatchResourcesListProjectsDurableSpaceIndexObjectTypes(t *testing.T) {
	// Start a test environment.
	ctx := context.Background()
	env := setupTestEnv(ctx, t)

	// Create a session and open its account.
	sessRef, _ := env.createSession(ctx, t)
	account := env.accessAccount(ctx, t, sessRef)

	// Create Spaces with durable index object types.
	createSpaceWithIndexObjectType(
		ctx,
		t,
		env,
		account,
		"glyph-canvas-space",
		"Canvas",
		"index",
		s4wave_canvas_world.CanvasTypeID,
	)
	createSpaceWithIndexObjectType(
		ctx,
		t,
		env,
		account,
		"glyph-layout-space",
		"Layout",
		"index",
		s4wave_layout_world.ObjectLayoutTypeID,
	)
	createSpaceWithIndexObjectType(
		ctx,
		t,
		env,
		account,
		"glyph-generic-space",
		"Generic",
		"",
		"",
	)

	// Watch the session resource list, including index types.
	resource := env.buildSessionResource(ctx, t, sessRef)
	stream := &captureResourcesListStream{
		ctx: ctx,
		stopWhen: func(resp *s4wave_session.WatchResourcesListResponse) bool {
			byName := indexObjectTypesBySpaceName(resp)
			_, hasGeneric := byName["Generic"]
			return len(byName) == 3 &&
				byName["Canvas"] != "" &&
				byName["Layout"] != "" &&
				hasGeneric
		},
	}

	// Capture one non-empty projection.
	err := resource.WatchResourcesList(&s4wave_session.WatchResourcesListRequest{IncludeIndexObjectTypes: true}, stream)
	if !errors.Is(err, errResourcesListProjectionCaptured) {
		t.Fatalf("WatchResourcesList returned %v, want capture sentinel", err)
	}
	if got := stream.nonEmptyProjectionResponses; got < 1 || got > 2 {
		t.Fatalf("responses with projected index ObjectTypes = %d, want one or two updates", got)
	}

	// Match each Space name to its index object type.
	got := indexObjectTypesBySpaceName(stream.response)
	want := map[string]string{
		"Canvas":  s4wave_canvas_world.CanvasTypeID,
		"Layout":  s4wave_layout_world.ObjectLayoutTypeID,
		"Generic": "",
	}
	if len(got) != len(want) {
		t.Fatalf("projected spaces = %#v, want %#v", got, want)
	}
	for name, wantType := range want {
		if gotType, ok := got[name]; !ok || gotType != wantType {
			t.Fatalf("index object type for %q = %q (present %v), want %q", name, gotType, ok, wantType)
		}
	}
}

// TestWatchResourcesListFirstTypeLatency measures first enrichment beside a slow
// mount in a 300-member local catalog. The gate adds 500 ms of mount latency.
func TestWatchResourcesListFirstTypeLatency(t *testing.T) {
	// Run the 300-Space measurement only when explicitly requested.
	if os.Getenv("SPACEWAVE_TEST_CATALOG_LATENCY") != "1" {
		t.Skip("set SPACEWAVE_TEST_CATALOG_LATENCY=1 for the 300-Space timing")
	}

	// Prepare durable indices before timing the Session stream.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	env := setupTestEnv(ctx, t)
	sessRef, _ := env.createSession(ctx, t)
	account := env.accessAccount(ctx, t, sessRef)
	createSpaceWithIndexObjectType(ctx, t, env, account, "projection-0", "projection-0", "index", s4wave_canvas_world.CanvasTypeID)
	createSpaceWithIndexObjectType(ctx, t, env, account, "projection-1", "projection-1", "index", s4wave_canvas_world.CanvasTypeID)

	// Populate the remaining catalog with generic Spaces, without pre-mounting bodies.
	for i := 2; i < 300; i++ {
		env.createSpaceOnAccount(ctx, t, account, "projection-"+strconv.Itoa(i))
	}

	// Delay one projection's first mount while all other Spaces read normally.
	sess, sessMount, err := session.ExMountSession(ctx, env.tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessMount.Release()
	release := make(chan struct{})
	gate := &resourcesListMountGate{Bus: env.tb.Bus, id: "projection-0", release: release}
	resource := resource_session.NewSessionResource(env.tb.Logger, gate, sess)
	started := time.Now()
	timer := time.AfterFunc(500*time.Millisecond, func() { close(release) })
	defer timer.Stop()
	stream := &captureResourcesListStream{
		ctx: ctx,
		stopWhen: func(resp *s4wave_session.WatchResourcesListResponse) bool {
			ready := 0
			for _, entry := range resp.GetSpacesList() {
				if entry.GetIndexObjectType() != "" {
					ready++
				}
			}
			return ready == 2
		},
	}

	// Retain the first arrival time while waiting for both durable types.
	err = resource.WatchResourcesList(&s4wave_session.WatchResourcesListRequest{IncludeIndexObjectTypes: true}, stream)
	if !errors.Is(err, errResourcesListProjectionCaptured) {
		t.Fatalf("WatchResourcesList: %v", err)
	}
	t.Logf("members=300 gate=500ms first-type=%s all-types=%s responses=%d mounts=%d", stream.firstTypeAt.Sub(started), time.Since(started), stream.responses, gate.mounts.Load())
}

// TestWatchResourcesListSlowSpaceKeepsFastType verifies a fast durable index
// arrives while another Space's first mount is still held.
func TestWatchResourcesListSlowSpaceKeepsFastType(t *testing.T) {
	// Prepare two durable Spaces on the existing Session testbed.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	env := setupTestEnv(ctx, t)
	sessRef, _ := env.createSession(ctx, t)
	account := env.accessAccount(ctx, t, sessRef)
	createSpaceWithIndexObjectType(ctx, t, env, account, "fast", "Fast", "index", s4wave_canvas_world.CanvasTypeID)
	createSpaceWithIndexObjectType(ctx, t, env, account, "slow", "Slow", "index", s4wave_layout_world.ObjectLayoutTypeID)

	// Keep the slow mount behind a gate until the fast type reaches the stream.
	sess, sessMount, err := session.ExMountSession(ctx, env.tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessMount.Release()
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	gate := &resourcesListMountGate{Bus: env.tb.Bus, id: "slow", release: release}
	resource := resource_session.NewSessionResource(env.tb.Logger, gate, sess)
	sawFast := false
	stream := &captureResourcesListStream{
		ctx: ctx,
		stopWhen: func(resp *s4wave_session.WatchResourcesListResponse) bool {
			types := indexObjectTypesBySpaceName(resp)
			if types["Fast"] == s4wave_canvas_world.CanvasTypeID && !sawFast {
				if types["Slow"] != "" {
					t.Error("slow type arrived before its mount was released")
				}
				sawFast = true
				releaseOnce.Do(func() { close(release) })
			}
			return sawFast && types["Slow"] == s4wave_layout_world.ObjectLayoutTypeID
		},
	}

	// Require both the independent fast update and the eventual slow update.
	err = resource.WatchResourcesList(&s4wave_session.WatchResourcesListRequest{IncludeIndexObjectTypes: true}, stream)
	if !errors.Is(err, errResourcesListProjectionCaptured) {
		t.Fatalf("WatchResourcesList: %v", err)
	}
	if !sawFast {
		t.Fatal("no independent fast type update")
	}
}

func createSpaceWithIndexObjectType(
	ctx context.Context,
	t *testing.T,
	env *testEnv,
	account *provider_local.ProviderAccount,
	id string,
	name string,
	indexPath string,
	typeID string,
) {
	// Report failures at the caller.
	t.Helper()

	// Create the Space SharedObject.
	meta, err := space.NewSharedObjectMeta(name)
	if err != nil {
		t.Fatalf("NewSharedObjectMeta(%q) failed: %v", name, err)
	}
	ref, err := account.CreateSharedObject(ctx, id, meta, "", "")
	if err != nil {
		t.Fatalf("CreateSharedObject(%q) failed: %v", name, err)
	}

	// Mount the Space body.
	mounted, mountedRef, err := space.ExMountSpaceSoBody(ctx, env.tb.Bus, ref, false, nil)
	if err != nil {
		t.Fatalf("ExMountSpaceSoBody(%q) failed: %v", name, err)
	}
	t.Cleanup(mountedRef.Release)

	// Skip the index write when the test Space has no index path.
	if indexPath == "" {
		return
	}

	// Write the index object type into the Space World.
	ws := world.NewEngineWorldState(mounted.GetSharedObjectBody().GetWorldEngine(), true)
	{
		createdObject, err := ws.CreateObject(ctx, indexPath, nil)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatalf("CreateObject(%q, %q) failed: %v", name, indexPath, err)
		}
	}
	if err := world_types.SetObjectType(ctx, ws, indexPath, typeID); err != nil {
		t.Fatalf("SetObjectType(%q, %q) failed: %v", name, typeID, err)
	}
	if _, _, err := space_world_ops.SetSpaceSettings(
		ctx,
		ws,
		"",
		"",
		&space_world.SpaceSettings{IndexPath: indexPath},
		true,
		time.Unix(1, 0),
	); err != nil {
		t.Fatalf("SetSpaceSettings(%q) failed: %v", name, err)
	}
}

func indexObjectTypesBySpaceName(resp *s4wave_session.WatchResourcesListResponse) map[string]string {
	byName := make(map[string]string, len(resp.GetSpacesList()))
	for _, entry := range resp.GetSpacesList() {
		byName[entry.GetSpaceMeta().GetName()] = entry.GetIndexObjectType()
	}
	return byName
}

type captureResourcesListStream struct {
	srpc.Stream
	ctx                         context.Context
	stopWhen                    func(*s4wave_session.WatchResourcesListResponse) bool
	response                    *s4wave_session.WatchResourcesListResponse
	nonEmptyProjectionResponses int
	// responses counts snapshots delivered through the real Session handler.
	responses int
	// firstTypeAt records when the stream first observes an enriched entry.
	firstTypeAt time.Time
}

func (s *captureResourcesListStream) Context() context.Context {
	return s.ctx
}

func (s *captureResourcesListStream) Send(resp *s4wave_session.WatchResourcesListResponse) error {
	// Record when the first projected type reaches the Session caller.
	s.responses++
	for _, entry := range resp.GetSpacesList() {
		if entry.GetIndexObjectType() != "" {
			s.nonEmptyProjectionResponses++
			if s.firstTypeAt.IsZero() {
				s.firstTypeAt = time.Now()
			}
			break
		}
	}

	// Retain the snapshot that ends this capture.
	if !s.stopWhen(resp) {
		return nil
	}
	s.response = resp.CloneVT()
	return errResourcesListProjectionCaptured
}

func (s *captureResourcesListStream) SendAndClose(resp *s4wave_session.WatchResourcesListResponse) error {
	return s.Send(resp)
}

func (s *captureResourcesListStream) MsgRecv(srpc.Message) error {
	return nil
}

func (s *captureResourcesListStream) MsgSend(srpc.Message) error {
	return nil
}

func (s *captureResourcesListStream) CloseSend() error {
	return nil
}

func (s *captureResourcesListStream) Close() error {
	return nil
}

var _ s4wave_session.SRPCSessionResourceService_WatchResourcesListStream = (*captureResourcesListStream)(nil)
