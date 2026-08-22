package sensor

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	policy "github.com/s4wave/spacewave/core/device/policy"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	"github.com/sirupsen/logrus"
)

func TestObjectKeyIsStableAndScoped(t *testing.T) {
	first := ObjectKey("devices/a", "radar")
	if first != ObjectKey("devices/a", "radar") {
		t.Fatal("ObjectKey is not stable for one device and endpoint")
	}
	if first == ObjectKey("devices/b", "radar") {
		t.Fatal("ObjectKey collides across devices")
	}
	if first == ObjectKey("devices/a", "other") {
		t.Fatal("ObjectKey collides across endpoints")
	}
}

func TestEndpointLabelNormalizesWhitespace(t *testing.T) {
	got := EndpointLabel("  living-room\tradar \t")
	if got != "living-room radar" {
		t.Fatalf("EndpointLabel() = %q, want %q", got, "living-room radar")
	}
}

// blockingDial blocks until the adapter context ends, so lifecycle tests join
// without real network activity.
func blockingDial(ctx context.Context, _ string) (net.Conn, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func newTestManager(t *testing.T) (*Manager, context.CancelFunc) {
	t.Helper()
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatalf("world testbed failed: %v", err)
	}
	t.Cleanup(wtb.Release)
	mgr := NewManager(
		logrus.WithField("test", t.Name()),
		wtb.Engine,
		"devices/test-device",
		blockingDial,
	)
	return mgr, wtb.Release
}

func endpoint(id string) *policy.SensorEndpointPolicy {
	return &policy.SensorEndpointPolicy{
		Id:          id,
		Enabled:     true,
		AdapterKind: s4wave_device.SensorAdapterKind_SENSOR_ADAPTER_KIND_ESPHOME,
		Endpoint:    "127.0.0.1:6053",
	}
}

func TestManagerReconcileStartsAndStopsAdapters(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	mgr.Reconcile(ctx, []*policy.SensorEndpointPolicy{endpoint("radar")})
	if _, ok := mgr.Status("radar"); !ok {
		t.Fatal("Status(radar) missing after Reconcile started the adapter")
	}

	mgr.Reconcile(ctx, nil)
	if _, ok := mgr.Status("radar"); ok {
		t.Fatal("Status(radar) still present after the endpoint left policy")
	}
}

func TestManagerReconcileRestartsAdapterOnConfigChange(t *testing.T) {
	mgr, releaseWorld := newTestManager(t)
	defer releaseWorld()
	ctx := context.Background()

	mgr.Reconcile(ctx, []*policy.SensorEndpointPolicy{endpoint("radar")})
	first := mgr.adapters["radar"]
	if first == nil {
		t.Fatal("adapter missing after Reconcile")
	}

	// The same endpoint id with a changed address must replace the old
	// adapter: reconciliation is level-triggered on the full config.
	changed := endpoint("radar")
	changed.Endpoint = "127.0.0.1:6054"
	mgr.Reconcile(ctx, []*policy.SensorEndpointPolicy{changed})
	second := mgr.adapters["radar"]
	if second == nil || second == first {
		t.Fatalf("adapter not replaced on address change: %v", mgr.adapters["radar"])
	}
	select {
	case <-first.done:
	case <-time.After(2 * time.Second):
		t.Fatal("replaced adapter did not stop")
	}

	// An unchanged config keeps the running adapter.
	before := mgr.adapters["radar"]
	mgr.Reconcile(ctx, []*policy.SensorEndpointPolicy{changed})
	if mgr.adapters["radar"] != before {
		t.Fatal("unchanged config restarted the adapter")
	}
}

func TestManagerReconcileSkipsDisabledAndUnknownKinds(t *testing.T) {
	mgr, _ := newTestManager(t)

	disabled := endpoint("spare")
	disabled.Enabled = false
	unknownKind := &policy.SensorEndpointPolicy{
		Id: "odd", Enabled: true, AdapterKind: s4wave_device.SensorAdapterKind_SENSOR_ADAPTER_KIND_UNKNOWN,
		Endpoint: "127.0.0.1:6054",
	}
	mgr.Reconcile(context.Background(), []*policy.SensorEndpointPolicy{disabled, unknownKind})
	if _, ok := mgr.Status("spare"); ok {
		t.Fatal("disabled endpoint started an adapter")
	}
	if _, ok := mgr.Status("odd"); ok {
		t.Fatal("unknown-kind endpoint started an adapter")
	}
}

func TestManagerCloseJoinsAdaptersAndRejectsReconcile(t *testing.T) {
	mgr, _ := newTestManager(t)

	mgr.Reconcile(context.Background(), []*policy.SensorEndpointPolicy{endpoint("radar")})
	mgr.Close()
	mgr.Close() // idempotent

	done := make(chan struct{})
	go func() {
		defer close(done)
		mgr.Reconcile(context.Background(), []*policy.SensorEndpointPolicy{endpoint("radar")})
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Reconcile after Close did not return")
	}
	if _, ok := mgr.Status("radar"); ok {
		t.Fatal("adapter present after Close")
	}
}

func TestAdapterCancellationJoinsRunLoop(t *testing.T) {
	mgr, releaseWorld := newTestManager(t)
	defer releaseWorld()

	runCtx, runCancel := context.WithCancel(context.Background())
	mgr.Reconcile(runCtx, []*policy.SensorEndpointPolicy{endpoint("radar")})

	status, ok := mgr.Status("radar")
	if !ok {
		t.Fatal("adapter missing after Reconcile")
	}

	runCancel()
	joined := make(chan struct{})
	go func() {
		mgr.mu.Lock()
		adapters := make([]*Adapter, 0, len(mgr.adapters))
		for _, a := range mgr.adapters {
			adapters = append(adapters, a)
		}
		mgr.mu.Unlock()
		for _, a := range adapters {
			<-a.done
		}
		close(joined)
	}()
	select {
	case <-joined:
	case <-time.After(2 * time.Second):
		t.Fatalf("adapter did not join after cancel; status %+v", status)
	}
}

// failingDial reports a refusal carrying a raw endpoint address, so the test
// can prove that text never reaches status or World state.
func failingDial(address string) DialFunc {
	return func(ctx context.Context, _ string) (net.Conn, error) {
		return nil, errors.Errorf("dial tcp %s: connect: connection refused", address)
	}
}

func TestAdapterFailureStoresSanitizedCategoryOnly(t *testing.T) {
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatalf("world testbed failed: %v", err)
	}
	defer wtb.Release()

	deviceKey := "devices/test-device"
	if err := createTestDevice(ctx, wtb.Engine, deviceKey); err != nil {
		t.Fatalf("create device object: %v", err)
	}

	rawAddress := "10.1.2.3:6053"
	mgr := NewManager(
		logrus.WithField("test", t.Name()),
		wtb.Engine,
		deviceKey,
		failingDial(rawAddress),
	)
	mgr.Reconcile(ctx, []*policy.SensorEndpointPolicy{endpoint("radar")})

	waitForState(t, mgr, "radar",
		s4wave_device.SensorConnectionState_SENSOR_CONNECTION_STATE_OFFLINE)

	status, _ := mgr.Status("radar")
	if status.LastError == "" || strings.Contains(status.LastError, rawAddress) {
		t.Fatalf("status LastError = %q, want a sanitized category without the raw address", status.LastError)
	}

	sensorKey := ObjectKey(deviceKey, "radar")
	var sensor *s4wave_device.Sensor
	deadline := time.Now().Add(2 * time.Second)
	for {
		sensor = readTestSensor(t, ctx, wtb.Engine, sensorKey)
		if sensor.GetConnectionState() == s4wave_device.SensorConnectionState_SENSOR_CONNECTION_STATE_OFFLINE {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("world connection_state = %v, want persisted OFFLINE", sensor.GetConnectionState())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !strings.Contains(sensor.GetEndpointLabel(), "radar") || strings.Contains(sensor.GetEndpointLabel(), rawAddress) {
		t.Fatalf("endpoint_label = %q, want sanitized label without raw address", sensor.GetEndpointLabel())
	}
	if strings.Contains(sensor.GetLastError(), rawAddress) {
		t.Fatalf("world last_error = %q, want a sanitized category without the raw address", sensor.GetLastError())
	}
}

func TestTransitionPersistsChangesAndDedupesUnchangedState(t *testing.T) {
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatalf("world testbed failed: %v", err)
	}
	defer wtb.Release()

	deviceKey := "devices/test-device"
	if err := createTestDevice(ctx, wtb.Engine, deviceKey); err != nil {
		t.Fatalf("create device object: %v", err)
	}

	// The clock advances on every read, so an unchanged-state write would
	// visibly move updated_at.
	step := 0
	mgr := NewManager(
		logrus.WithField("test", t.Name()),
		wtb.Engine,
		deviceKey,
		blockingDial,
	)
	mgr.now = func() time.Time {
		step++
		return time.Date(2026, 8, 22, 0, step, 0, 0, time.UTC)
	}
	adapter := newAdapter(mgr, ctx, endpoint("radar"))
	sensorKey := adapter.sensorKey

	connecting := s4wave_device.SensorConnectionState_SENSOR_CONNECTION_STATE_CONNECTING
	offline := s4wave_device.SensorConnectionState_SENSOR_CONNECTION_STATE_OFFLINE

	// First CONNECTING persists and creates the Sensor object.
	adapter.transition(connecting, "")
	first := readTestSensor(t, ctx, wtb.Engine, sensorKey)
	if first.GetConnectionState() != connecting {
		t.Fatalf("first transition state = %v, want CONNECTING", first.GetConnectionState())
	}

	// Repeating the unchanged CONNECTING retry write stays unwritten.
	adapter.transition(connecting, "")
	deduped := readTestSensor(t, ctx, wtb.Engine, sensorKey)
	if deduped.GetUpdatedAt().AsTime() != first.GetUpdatedAt().AsTime() {
		t.Fatalf("unchanged CONNECTING rewrote the object: updated_at moved to %v", deduped.GetUpdatedAt())
	}

	// An actual change to OFFLINE with its category does persist.
	adapter.transition(offline, "network error")
	third := readTestSensor(t, ctx, wtb.Engine, sensorKey)
	if third.GetConnectionState() != offline || third.GetLastError() != "network error" {
		t.Fatalf("world state = %v/%q, want OFFLINE/network error",
			third.GetConnectionState(), third.GetLastError())
	}
	if third.GetUpdatedAt().AsTime().Equal(first.GetUpdatedAt().AsTime()) {
		t.Fatal("actual OFFLINE change did not persist")
	}
}

// createTestDevice creates the Device object one graph edge needs.
func createTestDevice(ctx context.Context, engine world.Engine, deviceKey string) error {
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if _, _, err := world.CreateWorldObject(ctx, tx, deviceKey, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(&s4wave_device.Device{PeerId: "12D3KooWTestDevice"}, true)
		return nil
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// readTestSensor reads the current Sensor block from World state.
func readTestSensor(t *testing.T, ctx context.Context, engine world.Engine, key string) *s4wave_device.Sensor {
	t.Helper()
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("read transaction failed: %v", err)
	}
	defer tx.Discard()
	objState, found, err := tx.GetObject(ctx, key)
	if err != nil {
		t.Fatalf("get sensor object failed: %v", err)
	}
	if !found {
		t.Fatalf("sensor object %q missing", key)
	}
	sensor, err := readSensorBlock(ctx, objState)
	if err != nil {
		t.Fatalf("unmarshal sensor failed: %v", err)
	}
	return sensor
}

// waitForState waits until the endpoint's live status reaches the state.
func waitForState(
	t *testing.T,
	mgr *Manager,
	endpointID string,
	want s4wave_device.SensorConnectionState,
) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if status, ok := mgr.Status(endpointID); ok && status.ConnectionState == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	status, _ := mgr.Status(endpointID)
	t.Fatalf("status = %+v, want connection state %v", status, want)
}

func TestAdapterPersistRestoresDeviceEdgeIdempotently(t *testing.T) {
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatalf("world testbed failed: %v", err)
	}
	defer wtb.Release()

	deviceKey := "devices/test-device"

	// The graph edge requires both endpoints to exist as objects.
	createTx, err := wtb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("device create transaction failed: %v", err)
	}
	defer createTx.Discard()
	if _, _, err := world.CreateWorldObject(ctx, createTx, deviceKey, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(&s4wave_device.Device{PeerId: "12D3KooWTestDevice"}, true)
		return nil
	}); err != nil {
		t.Fatalf("create device object failed: %v", err)
	}
	if err := createTx.Commit(ctx); err != nil {
		t.Fatalf("commit device object failed: %v", err)
	}

	mgr := NewManager(
		logrus.WithField("test", t.Name()),
		wtb.Engine,
		deviceKey,
		blockingDial,
	)
	adapter := newAdapter(mgr, ctx, endpoint("radar"))
	sensorKey := ObjectKey(deviceKey, "radar")
	edge := func() world.GraphQuad {
		return s4wave_device.NewDeviceToSensorQuad(deviceKey, sensorKey)
	}
	edgePresent := func() bool {
		t.Helper()
		tx, err := wtb.Engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatalf("read transaction failed: %v", err)
		}
		defer tx.Discard()
		// The exact subject, predicate, and object identify one edge.
		quads, err := tx.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(deviceKey, s4wave_device.PredDeviceToSensor.String(), sensorKey, ""), 1)
		if err != nil {
			t.Fatalf("lookup quads failed: %v", err)
		}
		return len(quads) != 0
	}
	mutate := func(sensor *s4wave_device.Sensor) {
		sensor.ConnectionState = s4wave_device.SensorConnectionState_SENSOR_CONNECTION_STATE_CONNECTING
	}

	if err := adapter.persist(mutate); err != nil {
		t.Fatalf("first persist: %v", err)
	}
	if !edgePresent() {
		t.Fatal("device-to-sensor edge missing after create")
	}

	deleteTx, err := wtb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("write transaction failed: %v", err)
	}
	if err := deleteTx.DeleteGraphQuad(ctx, edge()); err != nil {
		t.Fatalf("delete edge failed: %v", err)
	}
	if err := deleteTx.Commit(ctx); err != nil {
		t.Fatalf("delete commit failed: %v", err)
	}
	if edgePresent() {
		t.Fatal("edge still present after deletion")
	}

	if err := adapter.persist(mutate); err != nil {
		t.Fatalf("second persist: %v", err)
	}
	if !edgePresent() {
		t.Fatal("persist did not restore the deleted device-to-sensor edge")
	}

	// A third persist over the present edge must stay a no-op write and keep
	// the link intact.
	if err := adapter.persist(mutate); err != nil {
		t.Fatalf("third persist: %v", err)
	}
	if !edgePresent() {
		t.Fatal("edge missing after no-op reconcile persist")
	}
}
