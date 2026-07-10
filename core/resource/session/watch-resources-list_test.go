package resource_session_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/srpc"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/space"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
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
	ctx := context.Background()
	env := setupTestEnv(ctx, t)
	_, spaceSobjectControllerRef, loadErr := env.tb.Bus.AddDirective(
		resolver.NewLoadControllerWithConfig(&space_sobject.Config{}),
		nil,
	)
	if loadErr != nil {
		t.Fatalf("load Space shared-object controller failed: %v", loadErr)
	}
	t.Cleanup(spaceSobjectControllerRef.Release)

	sessRef, _ := env.createSession(ctx, t)
	account := env.accessAccount(ctx, t, sessRef)

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

	err := resource.WatchResourcesList(&s4wave_session.WatchResourcesListRequest{IncludeIndexObjectTypes: true}, stream)
	if !errors.Is(err, errResourcesListProjectionCaptured) {
		t.Fatalf("WatchResourcesList returned %v, want capture sentinel", err)
	}
	if got := stream.nonEmptyProjectionResponses; got != 1 {
		t.Fatalf("responses with projected index ObjectTypes = %d, want 1 enriched snapshot", got)
	}

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
	t.Helper()

	meta, err := space.NewSharedObjectMeta(name)
	if err != nil {
		t.Fatalf("NewSharedObjectMeta(%q) failed: %v", name, err)
	}
	ref, err := account.CreateSharedObject(ctx, id, meta, "", "")
	if err != nil {
		t.Fatalf("CreateSharedObject(%q) failed: %v", name, err)
	}

	mounted, mountedRef, err := space.ExMountSpaceSoBody(ctx, env.tb.Bus, ref, false, nil)
	if err != nil {
		t.Fatalf("ExMountSpaceSoBody(%q) failed: %v", name, err)
	}
	t.Cleanup(mountedRef.Release)

	if indexPath == "" {
		return
	}

	ws := world.NewEngineWorldState(mounted.GetSharedObjectBody().GetWorldEngine(), true)
	if _, err := ws.CreateObject(ctx, indexPath, nil); err != nil {
		t.Fatalf("CreateObject(%q, %q) failed: %v", name, indexPath, err)
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
}

func (s *captureResourcesListStream) Context() context.Context {
	return s.ctx
}

func (s *captureResourcesListStream) Send(resp *s4wave_session.WatchResourcesListResponse) error {
	for _, entry := range resp.GetSpacesList() {
		if entry.GetIndexObjectType() != "" {
			s.nonEmptyProjectionResponses++
			break
		}
	}

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
