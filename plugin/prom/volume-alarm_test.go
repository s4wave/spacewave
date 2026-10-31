//go:build !js && !goscript

package prom_plugin

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	io_prometheus_client "github.com/aperturerobotics/go-prometheus-client-lite/client_model/go"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/prometheus/prometheus/model/labels"
	core_alarm_watch "github.com/s4wave/spacewave/core/alarm/watch"
	"github.com/s4wave/spacewave/core/metrics"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	db_kvtx "github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/volume"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_alarm "github.com/s4wave/spacewave/sdk/alarm"
	alarm_volume "github.com/s4wave/spacewave/sdk/alarm/volume"
	alarm_watch "github.com/s4wave/spacewave/sdk/alarm/watch"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	s4wave_function "github.com/s4wave/spacewave/sdk/function"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// alarmTargetKey is the device target the volume Alarms query.
const alarmTargetKey = "prom/target/device"

// TestVolumeAlarmGCStuck holds a real block pass in its mark, scrapes the
// process metrics into a device target, and checks that the stuck collection
// Alarm fires naming the volume through PromQL and the alarm watch, then
// clears once the pass ends.
func TestVolumeAlarmGCStuck(t *testing.T) {
	// Start the World and a volume on a store that can hold a block pass.
	e := newAlarmEnv(t)
	store := &gatedStore{Store: store_kvtx_inmem.NewStore()}
	v, err := volume_kvtx.NewVolume(e.ctx, "test/volume", store_kvkey.NewDefaultKVKey(), store, &store_kvtx.Config{}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = v.Close() })
	if _, _, err := v.PutBlock(e.ctx, []byte("one"), &block.PutOpts{RefsKnown: true}); err != nil {
		t.Fatal(err)
	}

	// Report the volume's health and start the watch.
	t.Cleanup(volume.RegisterHealth(v))
	def := alarm_volume.GCStuck(50 * time.Millisecond)
	e.startWatch(def)

	// An idle volume leaves the Alarm clear.
	e.scrape(time.Now())
	e.waitAlarm(def, "clear", func(a *s4wave_alarm.Alarm) bool {
		return a.GetState() == s4wave_alarm.AlarmState_ALARM_STATE_CLEAR && a.GetSummary() == "no volume is stuck in a collection phase"
	})

	// A pass held in its mark past the threshold fires the Alarm.
	started, release := store.arm(t)
	passErr := make(chan error, 1)
	go func() {
		passErr <- v.SweepUnreachedBlocks(e.ctx, v.GetRefGraph(), &block_gc.Stats{})
	}()
	<-started
	a := e.scrapeUntil(def, "firing", func(a *s4wave_alarm.Alarm) bool {
		return a.GetState() == s4wave_alarm.AlarmState_ALARM_STATE_FIRING
	})
	if a.GetSummary() != v.GetID()+" stuck in mark" || a.GetError() != "" {
		t.Fatalf("firing alarm: %v", a)
	}

	// The pass ending clears it.
	release()
	if err := <-passErr; err != nil {
		t.Fatal(err)
	}
	e.scrapeUntil(def, "cleared", func(a *s4wave_alarm.Alarm) bool {
		return a.GetState() == s4wave_alarm.AlarmState_ALARM_STATE_CLEAR && a.GetClearedMs() != 0
	})
}

// TestVolumeAlarmReserve scrapes a volume's free disk falling toward its
// reserve over half an hour, and checks that the forecast Alarm fires after
// its hold naming the hours left, and that the refusing writes Alarm fires
// once the volume reaches its reserve.
func TestVolumeAlarmReserve(t *testing.T) {
	// Start the World and a volume with ample free disk.
	e := newAlarmEnv(t)
	const gib = 1 << 30
	h := &fakeHealth{}
	h.set(10*gib, volume.VolumeWriteState_VOLUME_WRITE_STATE_WRITABLE)
	t.Cleanup(volume.RegisterHealth(h))

	// Start both reserve watches.
	defs := alarm_volume.Definitions(alarm_volume.Options{ReserveSoonHold: 100 * time.Millisecond})
	soon, reserve := defs[1], defs[2]
	e.startWatch(soon)
	e.startWatch(reserve)

	// One scrape forecasts nothing and leaves both Alarms clear.
	now := time.Now()
	e.scrape(now.Add(-30 * time.Minute))
	for _, def := range []*alarm_volume.Definition{soon, reserve} {
		e.waitAlarm(def, "clear", func(a *s4wave_alarm.Alarm) bool {
			return a.GetState() == s4wave_alarm.AlarmState_ALARM_STATE_CLEAR && a.GetError() == ""
		})
	}

	// Free disk falls 1 GiB each 10 minutes to 3 GiB above the reserve, and
	// the volume then refuses writes.
	for i, free := range []uint64{9 * gib, 8 * gib, 7 * gib} {
		state := volume.VolumeWriteState_VOLUME_WRITE_STATE_WRITABLE
		if i == 2 {
			state = volume.VolumeWriteState_VOLUME_WRITE_STATE_RESERVE
		}
		h.set(free, state)
		e.scrape(now.Add(time.Duration(i-2) * 10 * time.Minute))
	}

	// The forecast reaches the reserve in half an hour, and both Alarms fire.
	a := e.waitAlarm(soon, "firing", func(a *s4wave_alarm.Alarm) bool {
		return a.GetState() == s4wave_alarm.AlarmState_ALARM_STATE_FIRING
	})
	if a.GetSummary() != "test/volume reaches its reserve in 1 h" || a.GetFiredMs()-a.GetSinceMs() < 100 {
		t.Fatalf("forecast alarm: %v", a)
	}
	a = e.waitAlarm(reserve, "firing", func(a *s4wave_alarm.Alarm) bool {
		return a.GetState() == s4wave_alarm.AlarmState_ALARM_STATE_FIRING
	})
	if a.GetSummary() != "test/volume refuses writes below its reserve" {
		t.Fatalf("reserve alarm: %v", a)
	}
}

// alarmEnv is a World with a device target, its PromQL service and the alarm
// watch factory.
type alarmEnv struct {
	// t is the test.
	t *testing.T
	// ctx is the test context.
	ctx context.Context
	// tb is the World testbed.
	tb *world_testbed.Testbed
	// store writes the device target.
	store *Store
	// sc scrapes the process metrics into store.
	sc *scraper
}

// newAlarmEnv starts a World serving PromQL for the target type, creates the
// device target and its scraper, and registers the watch factory.
func newAlarmEnv(t *testing.T) *alarmEnv {
	// Start the World with the watch factory and the target type.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	tb := world_testbed.MustDefault(t, ctx)
	tb.StaticResolver.AddFactory(core_alarm_watch.NewFactory(tb.Bus))
	release, err := tb.Bus.AddController(ctx, targetTypes{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	// Create the device target and its scraper.
	store, err := CreateStore(ctx, tb.Engine, alarmTargetKey, labels.FromStrings("job", "spacewave"))
	if err != nil {
		t.Fatal(err)
	}
	gather := func(context.Context) ([]*io_prometheus_client.MetricFamily, error) { return metrics.Gather() }
	cfg := &s4wave_prom.ScrapeConfig{Url: s4wave_prom.DeviceMetricsURL}
	sc := newScraper(logrus.NewEntry(logrus.New()), store, cfg, "peer-a", time.Minute, gatherSource(gather))
	return &alarmEnv{t: t, ctx: ctx, tb: tb, store: store, sc: sc}
}

// startWatch stores the definition's function and starts its watch over the
// device target.
func (e *alarmEnv) startWatch(def *alarm_volume.Definition) {
	// Store the function.
	e.t.Helper()
	fn, err := def.Function()
	if err != nil {
		e.t.Fatal(err)
	}
	functionKey := "functions/" + def.Name
	err = world.ExecTransaction(e.ctx, e.tb.Engine, true, func(ctx context.Context, tx world.WorldState) error {
		return s4wave_function.CreatePureFunction(ctx, tx, functionKey, fn)
	})
	if err != nil {
		e.t.Fatal(err)
	}

	// Build the watch from the definition's node and wait for it to run.
	conf, err := alarm_watch.NewConfigFromNode(&s4wave_flowgraph.PlacedFlowgraphNode{
		EngineID: e.tb.EngineID,
		Node:     &s4wave_flowgraph.FlowgraphNode{ObjectKey: "alarms/" + def.Name, Parameters: def.NodeParameters(functionKey, alarmTargetKey)},
	})
	if err != nil {
		e.t.Fatal(err)
	}
	_, _, ref, err := loader.WaitExecControllerRunning(e.ctx, e.tb.Bus, resolver.NewLoadControllerWithConfig(conf), nil)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(ref.Release)
}

// scrape scrapes the process metrics stamped at ts and commits them.
func (e *alarmEnv) scrape(ts time.Time) {
	e.t.Helper()
	e.sc.scrape(e.ctx, ts)
	if _, err := CommitStores(e.ctx, e.tb.Engine, e.store); err != nil {
		e.t.Fatal(err)
	}
}

// readAlarm reads the definition's Alarm, nil before the watch writes it.
func (e *alarmEnv) readAlarm(def *alarm_volume.Definition) *s4wave_alarm.Alarm {
	e.t.Helper()
	a, err := s4wave_alarm.ReadAlarm(e.ctx, e.tb.WorldState, "alarms/"+def.Name)
	if err != nil {
		e.t.Fatal(err)
	}
	return a
}

// waitAlarm waits until the definition's Alarm exists and done accepts it,
// and returns it.
func (e *alarmEnv) waitAlarm(def *alarm_volume.Definition, what string, done func(a *s4wave_alarm.Alarm) bool) *s4wave_alarm.Alarm {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(e.ctx, 10*time.Second)
	defer cancel()
	for {
		// Read the Alarm at the current sequence number.
		seqno, err := e.tb.WorldState.GetSeqno(ctx)
		if err != nil {
			e.t.Fatal(err)
		}
		if a := e.readAlarm(def); a != nil && done(a) {
			return a
		}

		// Wait for the next commit.
		if _, err := e.tb.WorldState.WaitSeqno(ctx, seqno+1); err != nil {
			e.t.Fatalf("waiting for the %s %s alarm: %v, last %v", what, def.Name, err, e.readAlarm(def))
		}
	}
}

// scrapeUntil scrapes every 100 ms, as a scraper on a short interval would,
// until the definition's Alarm exists and done accepts it, and returns it.
func (e *alarmEnv) scrapeUntil(def *alarm_volume.Definition, what string, done func(a *s4wave_alarm.Alarm) bool) *s4wave_alarm.Alarm {
	// Scrape now and then at each tick until the deadline.
	e.t.Helper()
	deadline := time.After(10 * time.Second)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		// Scrape and check the Alarm.
		e.scrape(time.Now())
		if a := e.readAlarm(def); a != nil && done(a) {
			return a
		}

		// Scrape again at the next tick.
		select {
		case <-deadline:
			e.t.Fatalf("waiting for the %s %s alarm, last %v", what, def.Name, e.readAlarm(def))
		case <-tick.C:
		}
	}
}

// gatedStore holds the first write transaction opened after arm until the
// test releases it.
type gatedStore struct {
	db_kvtx.Store
	// mu guards started and release.
	mu sync.Mutex
	// started is closed when the held transaction opens, nil while not armed.
	started chan struct{}
	// release ends the hold.
	release chan struct{}
}

// SupportsAtomicCommit advertises the in-memory store's atomic commits.
func (s *gatedStore) SupportsAtomicCommit() bool { return true }

// arm holds the next write transaction, returning a channel closed once it
// opens and a func that releases it, which also runs when the test ends.
func (s *gatedStore) arm(t *testing.T) (<-chan struct{}, func()) {
	// Set a fresh hold whose release runs once.
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started, s.release = make(chan struct{}), make(chan struct{})
	release := s.release
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	return s.started, unblock
}

// NewTransaction holds the armed write transaction until its release.
func (s *gatedStore) NewTransaction(ctx context.Context, write bool) (db_kvtx.Tx, error) {
	// Take the hold if armed.
	var started, release chan struct{}
	if write {
		s.mu.Lock()
		started, release = s.started, s.release
		s.started, s.release = nil, nil
		s.mu.Unlock()
	}

	// Wait for the release.
	if started != nil {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.Store.NewTransaction(ctx, write)
}

// fakeHealth reports a volume's free disk and write state against a 4 GiB
// reserve.
type fakeHealth struct {
	// mu guards h and wake.
	mu sync.Mutex
	// h is the current health.
	h *volume.VolumeHealth
	// wake is closed at the next change.
	wake chan struct{}
}

// set replaces the free disk and write state and wakes the watchers.
func (f *fakeHealth) set(free uint64, state volume.VolumeWriteState) {
	// Replace the health under the lock.
	f.mu.Lock()
	defer f.mu.Unlock()
	f.h = &volume.VolumeHealth{
		VolumeId:      "test/volume",
		GcPhase:       volume.VolumeGCPhase_VOLUME_GC_PHASE_IDLE,
		GcPhaseStart:  timestamppb.Now(),
		FreeDiskBytes: free,
		ReserveBytes:  4 << 30,
		SizesKnown:    true,
		SizesAt:       timestamppb.Now(),
		WriteState:    state,
	}

	// Wake the watchers of the previous health.
	if f.wake != nil {
		close(f.wake)
	}
	f.wake = make(chan struct{})
}

// GetHealthSnapshotWithWait returns the current health and a channel closed
// at the next change.
func (f *fakeHealth) GetHealthSnapshotWithWait() (*volume.VolumeHealth, <-chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.h.CloneVT(), f.wake
}

// targetTypes serves the PromQL service for every target object.
type targetTypes struct{}

// GetControllerInfo returns information about the controller.
func (targetTypes) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/prom-target-types", controller.MustParseVersion("0.0.1"), "prom target object type")
}

// Execute returns at once, since the controller only handles directives.
func (targetTypes) Execute(context.Context) error {
	return nil
}

// HandleDirective answers the lookup of the target type.
func (c targetTypes) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(objecttype.LookupObjectType)
	if !ok || dir.LookupObjectTypeID() != s4wave_prom.TargetTypeID {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]objecttype.LookupObjectTypeValue{objecttype.NewObjectType(s4wave_prom.TargetTypeID, c.newResource)}), nil)
}

// newResource serves PromQL over the engine's targets.
func (targetTypes) newResource(_ context.Context, _ *logrus.Entry, _ bus.Bus, eng world.Engine, _ world.WorldState, _ string) (srpc.Invoker, func(), error) {
	mux := srpc.NewMux()
	svc := &queryService{engine: newQueryEngine(), eng: eng, q: NewQueryable(eng, worldTargets{eng: eng})}
	if err := s4wave_prom.SRPCRegisterPromQueryResourceService(mux, svc); err != nil {
		return nil, nil, err
	}
	return mux, func() {}, nil
}

// Close releases nothing.
func (targetTypes) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ controller.Controller = targetTypes{}
	_ volume.HealthWatcher  = (*fakeHealth)(nil)
)
