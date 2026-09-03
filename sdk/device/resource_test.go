package s4wave_device

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// fixtureServiceID is the SRPC service id stamped into the test capability
// link and served by the fixture protocol service.
const fixtureServiceID = "plugin/test-plugin/fixture/v0"

// testDeviceObjectKey is the Device object key used by the tests.
const testDeviceObjectKey = "devices/test-device"

// fixtureProtocolService serves one request/reply method for the fixture
// service id so tests can prove the child resource reaches the looked-up
// service.
var fixtureProtocolService = srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	if serviceID != fixtureServiceID {
		return false, nil
	}
	req := &WatchDeviceStateRequest{}
	if err := strm.MsgRecv(req); err != nil {
		return true, err
	}
	return true, strm.MsgSend(&WatchDeviceStateResponse{State: &Device{PeerId: "fixture"}})
})

// fixtureProtocolController resolves LookupRpcService demands for the fixture
// service and records how many lookup demands open and close.
type fixtureProtocolController struct {
	opened chan struct{}
	closed chan struct{}
}

// newFixtureProtocolController constructs the fixture controller.
func newFixtureProtocolController() *fixtureProtocolController {
	return &fixtureProtocolController{
		opened: make(chan struct{}, 8),
		closed: make(chan struct{}, 8),
	}
}

// GetControllerInfo returns the controller identity.
func (f *fixtureProtocolController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"test/device-capability-fixture",
		controller.MustParseVersion("0.0.1"),
		"fixture capability protocol service",
	)
}

// Execute blocks until the controller is removed.
func (f *fixtureProtocolController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// HandleDirective resolves fixture service lookups with a standing value and
// records when the lookup demand starts and ends.
func (f *fixtureProtocolController) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		if d.LookupRpcServiceID() != fixtureServiceID {
			return nil, nil
		}
		return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
			f.opened <- struct{}{}
			handler.AddValue(fixtureProtocolService)
			<-ctx.Done()
			f.closed <- struct{}{}
			return nil
		}), nil)
	default:
		return nil, nil
	}
}

// _ is a type assertion
var _ controller.Controller = (*fixtureProtocolController)(nil)

// writeDeviceObject writes or updates the Device block in one transaction.
func writeDeviceObject(t *testing.T, ctx context.Context, engine world.Engine, dev *Device) {
	t.Helper()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction: %v", err)
	}
	defer tx.Discard()

	if _, _, err := world.CreateWorldObject(ctx, tx, testDeviceObjectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(dev, true)
		return nil
	}); err != nil {
		t.Fatalf("CreateWorldObject: %v", err)
	}

	objState, found, err := tx.GetObject(ctx, testDeviceObjectKey)
	if err != nil || !found {
		t.Fatalf("GetObject after create: found=%v err=%v", found, err)
	}
	if _, _, err := world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(dev, true)
		return nil
	}); err != nil {
		t.Fatalf("SetBlock: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// startDeviceStack registers the DeviceResource on a fresh Resource RPC stack
// and returns the Device service client plus a cleanup function.
func startDeviceStack(t *testing.T, ctx context.Context, r *DeviceResource) (SRPCDeviceResourceServiceClient, func()) {
	t.Helper()
	rootMux := srpc.NewMux()
	if err := SRPCRegisterDeviceResourceService(rootMux, r); err != nil {
		t.Fatalf("register device service: %v", err)
	}
	server := resource_server.NewResourceServer(rootMux)
	serverMux := srpc.NewMux()
	if err := server.Register(serverMux); err != nil {
		t.Fatalf("register resource server: %v", err)
	}
	service := resource_server.NewTestServiceClientForTest(serverMux)
	client, err := resource_client.NewClient(ctx, service)
	if err != nil {
		t.Fatalf("resource client: %v", err)
	}
	rootRef := client.AccessRootResource()
	deviceClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		client.Release()
		t.Fatalf("device client: %v", err)
	}
	return NewSRPCDeviceResourceServiceClient(deviceClient), func() {
		rootRef.Release()
		client.Release()
	}
}

// invokeFixture calls one request/reply method on the stamped service through
// the given child client.
func invokeFixture(t *testing.T, ctx context.Context, client srpc.Client) {
	t.Helper()
	out := &WatchDeviceStateResponse{}
	if err := client.ExecCall(ctx, fixtureServiceID, "Ping", &WatchDeviceStateRequest{}, out); err != nil {
		t.Fatalf("fixture call: %v", err)
	}
	if out.GetState().GetPeerId() != "fixture" {
		t.Fatalf("fixture peer id = %q, want fixture", out.GetState().GetPeerId())
	}
}

// waitChannelClosed fails the test when ch does not receive before timeout.
func waitChannelClosed(t *testing.T, ch chan struct{}, timeout time.Duration, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %s", name)
	}
}

// newCapabilityTestDevice builds a selectable Device declaring one
// stream-backed capability with the requested state.
func newCapabilityTestDevice(state DeviceCapabilityState) *Device {
	return &Device{
		PeerId:     "12D3KooWDevice",
		Label:      "Build Host",
		SetupState: DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		Capabilities: []*DeviceCapability{
			{
				Id:    "glados",
				Kind:  "sensor",
				Label: "Fixture Capability",
				State: state,
				Link: &DeviceCapabilityLink{
					ProtocolId: fixtureServiceID,
				},
				Policy: &DeviceCapabilityPolicy{
					LocalState: DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
					GrantState: DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
				},
			},
		},
	}
}

// TestDeviceAccessCapabilityLifecycle proves fresh-state admission, retained
// child lifetime across revocation, exact resource-id resolution before and
// after release, and exactly once lookup cleanup.
func TestDeviceAccessCapabilityLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	fixture := newFixtureProtocolController()
	relFixture, err := wtb.Bus.AddController(ctx, fixture, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relFixture)

	writeDeviceObject(t, ctx, wtb.Engine, newCapabilityTestDevice(DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE))

	// The constructor state intentionally still reports AVAILABLE after the
	// world block is revoked below; every admission must reread the block.
	r := NewDeviceResource(nil, wtb.Bus, wtb.Engine, world.NewEngineWorldState(wtb.Engine, true), testDeviceObjectKey, newCapabilityTestDevice(DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE))
	deviceSvc, resClient, cleanupStack := startDeviceStack(t, ctx, r)
	t.Cleanup(cleanupStack)

	// Open one lease and prove the child reaches the stamped service.
	resp, err := deviceSvc.AccessCapability(ctx, &AccessCapabilityRequest{CapabilityId: "glados"})
	if err != nil {
		t.Fatalf("AccessCapability: %v", err)
	}
	if resp.GetResourceId() == 0 {
		t.Fatal("expected nonzero resource id")
	}
	if resp.GetCapability().GetId() != "glados" {
		t.Fatalf("capability id = %q, want glados", resp.GetCapability().GetId())
	}
	if len(fixture.opened) != 1 {
		t.Fatalf("lookup opens = %d, want 1", len(fixture.opened))
	}

	childRef := resClient.CreateResourceReference(resp.GetResourceId())
	childClient, err := childRef.GetClient()
	if err != nil {
		t.Fatalf("child client: %v", err)
	}
	invokeFixture(t, ctx, childClient)

	// Revoke the capability in the world block. The cached constructor state
	// still reports AVAILABLE, so denial proves the fresh-state read.
	writeDeviceObject(t, ctx, wtb.Engine, newCapabilityTestDevice(DeviceCapabilityState_DEVICE_CAPABILITY_STATE_GRANT_BLOCKED))
	if _, err := deviceSvc.AccessCapability(ctx, &AccessCapabilityRequest{CapabilityId: "glados"}); err == nil {
		t.Fatal("expected revoked capability admission to fail")
	}

	// A pre-revocation returned resource stays valid until explicit release.
	invokeFixture(t, ctx, childClient)

	// The exact resource id still resolves before release.
	preReleaseRef := resClient.CreateResourceReference(resp.GetResourceId())
	if _, err := preReleaseRef.GetClient(); err != nil {
		t.Fatalf("pre-release resolution: %v", err)
	}
	preReleaseRef.Release()

	// Release cleans the underlying lookup exactly once.
	childRef.Release()
	waitChannelClosed(t, fixture.closed, 5*time.Second, "fixture lookup close")
	select {
	case <-fixture.closed:
		t.Fatal("lookup closed more than once for one lease")
	case <-time.After(250 * time.Millisecond):
	}
	if len(fixture.opened) != 1 || len(fixture.closed) != 1 {
		t.Fatalf("cleanup counts: opened=%d closed=%d, want 1/1", len(fixture.opened), len(fixture.closed))
	}

	// The released resource id no longer resolves.
	releasedRef := resClient.CreateResourceReference(resp.GetResourceId())
	if _, err := releasedRef.GetClient(); err == nil {
		releasedRef.Release()
		t.Fatal("expected released resource id to fail resolution")
	}

	// Post-revocation admission stays denied from fresh state.
	if _, err := deviceSvc.AccessCapability(ctx, &AccessCapabilityRequest{CapabilityId: "glados"}); err == nil {
		t.Fatal("expected post-revocation admission to fail")
	}
	if len(fixture.opened) != 1 {
		t.Fatalf("denied admissions must not open lookups, opened=%d", len(fixture.opened))
	}

	// Restore availability and prove re-admission opens one fresh lookup.
	writeDeviceObject(t, ctx, wtb.Engine, newCapabilityTestDevice(DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE))
	resp2, err := deviceSvc.AccessCapability(ctx, &AccessCapabilityRequest{CapabilityId: "glados"})
	if err != nil {
		t.Fatalf("re-admission: %v", err)
	}
	ref2 := resClient.CreateResourceReference(resp2.GetResourceId())
	ref2.Release()
	waitChannelClosed(t, fixture.closed, 5*time.Second, "second lookup close")
}

// TestDeviceAccessCapabilityIgnoresCachedRevokedState proves admission reads
// the engine-backed Device block, never the WatchDeviceState cache: a resource
// constructed with a revoked snapshot admits against a selectable world block.
func TestDeviceAccessCapabilityIgnoresCachedRevokedState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	fixture := newFixtureProtocolController()
	relFixture, err := wtb.Bus.AddController(ctx, fixture, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relFixture)

	writeDeviceObject(t, ctx, wtb.Engine, newCapabilityTestDevice(DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE))

	r := NewDeviceResource(nil, wtb.Bus, wtb.Engine, world.NewEngineWorldState(wtb.Engine, true), testDeviceObjectKey, newCapabilityTestDevice(DeviceCapabilityState_DEVICE_CAPABILITY_STATE_GRANT_BLOCKED))
	deviceSvc, resClient, cleanupStack := startDeviceStack(t, ctx, r)
	t.Cleanup(cleanupStack)

	resp, err := deviceSvc.AccessCapability(ctx, &AccessCapabilityRequest{CapabilityId: "glados"})
	if err != nil {
		t.Fatalf("AccessCapability: %v", err)
	}
	if len(fixture.opened) != 1 {
		t.Fatalf("lookup opens = %d, want 1", len(fixture.opened))
	}
	ref := resClient.CreateResourceReference(resp.GetResourceId())
	ref.Release()
	waitChannelClosed(t, fixture.closed, 5*time.Second, "fixture lookup close")
}

// TestDeviceAccessCapabilityCancelDoesNotLeakLookup proves a canceled waitOne
// lookup returns an error without opening or leaking the service demand, and a
// later call succeeds.
func TestDeviceAccessCapabilityCancelDoesNotLeakLookup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	fixture := newFixtureProtocolController()
	relFixture, err := wtb.Bus.AddController(ctx, fixture, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relFixture)

	writeDeviceObject(t, ctx, wtb.Engine, newCapabilityTestDevice(DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE))

	r := NewDeviceResource(nil, wtb.Bus, wtb.Engine, world.NewEngineWorldState(wtb.Engine, true), testDeviceObjectKey, nil)
	deviceSvc, resClient, cleanupStack := startDeviceStack(t, ctx, r)
	t.Cleanup(cleanupStack)

	callCtx, callCancel := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() {
		_, uerr := deviceSvc.AccessCapability(callCtx, &AccessCapabilityRequest{CapabilityId: "glados"})
		errCh <- uerr
	}()

	// The lookup waits for the service instead of failing immediately.
	select {
	case err := <-errCh:
		t.Fatalf("canceled-phase call returned early: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	callCancel()
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected canceled call to fail")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for canceled call")
	}
	if len(fixture.opened) != 0 {
		t.Fatalf("canceled wait must not open lookups, opened=%d", len(fixture.opened))
	}

	relFixture()
	fixture2 := newFixtureProtocolController()
	relFixture2, err := wtb.Bus.AddController(ctx, fixture2, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relFixture2)

	resp, err := deviceSvc.AccessCapability(ctx, &AccessCapabilityRequest{CapabilityId: "glados"})
	if err != nil {
		t.Fatalf("post-cancel admission: %v", err)
	}
	ref := resClient.CreateResourceReference(resp.GetResourceId())
	ref.Release()
	waitChannelClosed(t, fixture2.closed, 5*time.Second, "fixture lookup close")
}

// TestDeviceAccessCapabilityRequiresProtocolLink proves link validation runs
// before any service lookup or handle open.
func TestDeviceAccessCapabilityRequiresProtocolLink(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	fixture := newFixtureProtocolController()
	relFixture, err := wtb.Bus.AddController(ctx, fixture, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relFixture)

	dev := newCapabilityTestDevice(DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE)
	dev.Capabilities[0].Link = &DeviceCapabilityLink{ObjectKey: "some/object", TypeId: "some/type"}
	writeDeviceObject(t, ctx, wtb.Engine, dev)

	r := NewDeviceResource(nil, wtb.Bus, wtb.Engine, world.NewEngineWorldState(wtb.Engine, true), testDeviceObjectKey, nil)
	deviceSvc, _, cleanupStack := startDeviceStack(t, ctx, r)
	t.Cleanup(cleanupStack)

	if _, err := deviceSvc.AccessCapability(ctx, &AccessCapabilityRequest{CapabilityId: "glados"}); err == nil {
		t.Fatal("expected missing protocol link to fail")
	}
	if _, err := deviceSvc.AccessCapability(ctx, &AccessCapabilityRequest{CapabilityId: ""}); err == nil {
		t.Fatal("expected empty capability id to fail")
	}
	if _, err := deviceSvc.AccessCapability(ctx, &AccessCapabilityRequest{CapabilityId: "other"}); err == nil {
		t.Fatal("expected unknown capability id to fail")
	}
	if len(fixture.opened) != 0 {
		t.Fatalf("failed validation must not open lookups, opened=%d", len(fixture.opened))
	}
}
