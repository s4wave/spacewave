package space_world_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// testOpPlugin is the plugin the World lists that handles the mock World
// operation.
const testOpPlugin = "test-plugin"

// pluginOpSpace is a Space whose order ends in an operation only the test
// plugin resolves, served by an engine started while the plugin ran.
type pluginOpSpace struct {
	f      *adapterFixture
	eng    *world.BusEngine
	engRef directive.Reference
	// marks records the replay mark of each run of the plugin's operations.
	marks *replayMarks
	// authored is the number of runs before any replay.
	authored int
	// seqno is the World seqno after the operation.
	seqno uint64
	// start starts the plugin once the loader asked for it.
	start chan struct{}
	// loads receives the plugins of the loader's first request.
	loads chan []string
}

// newPluginOpSpace writes, while the test plugin runs, an object, then an
// operation only the plugin resolves, then stops the plugin and adds a loader
// that starts it again on request once start closes. With approve set the
// loader reports the plugin awaiting approval until approve closes. A write
// of the operation on a build missing on this device fails first and leaves
// no record in the order.
func newPluginOpSpace(ctx context.Context, t *testing.T, approve chan struct{}) *pluginOpSpace {
	// Resolve the mock World operation only through the plugin, which the
	// World lists.
	t.Helper()
	f := newAdapterFixtureOn(t, fixtureOpts{opTypes: &space_world.OpTypes{Lookup: optypes.LookupWorldOp}})
	s := &pluginOpSpace{f: f, marks: &replayMarks{}, start: make(chan struct{}), loads: make(chan []string, 1)}

	// Write the object, then the operation, while the plugin runs.
	releasePlugin := startTestOpPlugin(ctx, t, f.tb.Bus, s.marks)
	s.eng, s.engRef = f.start(t)
	listTestOpPlugin(ctx, t, s.eng)
	err := world.ExecTransaction(ctx, s.eng, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, "before-object", nil)
		world.ReleaseObjectState(obj)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Fail a write on a build missing on this device, then write the
	// operation. The failed write left no record before it, so the order
	// grows by this write's operation alone.
	so := f.space(t)
	before := opCount(ctx, t, so)
	write := func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, "plugin-object", nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		op := &markedOp{Operation: world_mock.NewMockWorldOp("plugin-object", "hello"), marks: s.marks}
		_, _, err = ws.ApplyWorldOp(ctx, op, "")
		return err
	}
	s.marks.missing.Store(1)
	var missing *world.PluginExecutableMissingError
	if err := world.ExecTransaction(ctx, s.eng, true, write); !errors.As(err, &missing) {
		t.Fatalf("write on a missing build = %v, want PluginExecutableMissingError", err)
	}
	if err := world.ExecTransaction(ctx, s.eng, true, write); err != nil {
		t.Fatal(err)
	}
	waitOps(ctx, t, so, before+1)
	if n := opCount(ctx, t, so); n != before+1 {
		t.Fatalf("operations = %d, want %d: the failed write left a record", n, before+1)
	}
	if got := s.marks.get(); len(got) == 0 || got[0] {
		t.Fatalf("author's run replay marks = %v, want the first unmarked", got)
	}
	s.authored = len(s.marks.get())
	if s.seqno, err = s.eng.GetSeqno(ctx); err != nil {
		t.Fatal(err)
	}

	// Stop the plugin, and add a loader that starts it on request.
	releasePlugin()
	releaseLoader, err := f.tb.Bus.AddController(ctx, &testOpPluginLoader{b: f.tb.Bus, engineID: f.engineID, loads: s.loads, start: s.start, approve: approve, marks: s.marks}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseLoader)
	return s
}

// remountWaiting remounts the engine without the plugin: replay stops at the
// operation and asks for the plugin the World lists, and the remounted engine
// serves the World before the operation while it waits.
func (s *pluginOpSpace) remountWaiting(ctx context.Context, t *testing.T) {
	// Remount: the engine serves while replay waits.
	t.Helper()
	s.eng, s.engRef = s.f.remount(ctx, t, s.eng, s.engRef)

	// The adapter asks for the plugin once.
	select {
	case ids := <-s.loads:
		if !slices.Equal(ids, []string{testOpPlugin}) {
			t.Fatalf("replay asked for plugins %q, want %s", ids, testOpPlugin)
		}
	case <-ctx.Done():
		t.Fatal("replay stopped at the plugin's operation without asking for the plugin")
	}

	// Reads are served at the World before the operation.
	seqno, err := s.eng.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if seqno >= s.seqno {
		t.Fatalf("waiting engine serves seqno %d, want one before the operation's %d", seqno, s.seqno)
	}
	if err := world.AccessEngine(ctx, s.eng, func(ctx context.Context, eng world.Engine) error {
		return hasObjects(ctx, eng, "before-object")
	}); err != nil {
		t.Fatal(err)
	}
}

// runPluginOpThrough writes an operation whose type only a plugin resolves,
// then remounts without the plugin: replay stops at the operation, asks for
// the plugin the World lists with LoadOpPlugins, serves the World before the
// operation meanwhile, and continues once the plugin's handler registers, to
// the seqno the World had before the remount. The author's run of the
// operation carries no replay mark, and replay's run carries it. Replay's
// first run meets a build missing on this device, and its second writes an
// object and then fails after it was sent: each stops replay at the operation
// without a stored refusal, and the retry applies on the World as it was
// before that attempt, so the result equals one clean apply.
func runPluginOpThrough(t *testing.T) {
	// Remount with replay waiting at the operation.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	s := newPluginOpSpace(ctx, t, nil)
	s.remountWaiting(ctx, t)
	defer func() { s.engRef.Release() }()

	// Register the handler, failing its first two replayed runs: replay
	// reaches the World after the operation.
	s.marks.missing.Store(1)
	s.marks.crashes.Store(1)
	close(s.start)
	if _, err := s.eng.WaitSeqno(ctx, s.seqno); err != nil {
		t.Fatal(err)
	}
	if seqno, err := s.eng.GetSeqno(ctx); err != nil || seqno != s.seqno {
		t.Fatalf("seqno after replay = %d, %v, want %d", seqno, err, s.seqno)
	}

	// The failed attempt's object is gone, as after one clean apply.
	if err := world.AccessEngine(ctx, s.eng, func(ctx context.Context, eng world.Engine) error {
		if err := hasObjects(ctx, eng, "plugin-object"); err != nil {
			return err
		}
		return lacksObject(ctx, eng, crashObject)
	}); err != nil {
		t.Fatal(err)
	}
	if got := s.marks.get()[s.authored:]; len(got) == 0 || slices.Contains(got, false) {
		t.Fatalf("replay's runs replay marks = %v, want every one marked", got)
	}
	if n := s.marks.missing.Load(); n != 0 {
		t.Fatalf("replay never met the missing build: %d left", n)
	}
	if n := s.marks.crashes.Load(); n != 0 {
		t.Fatalf("replay never met the failing handler: %d left", n)
	}
}

// crashObject is the object a failing run of the plugin's handler writes
// before it fails.
const crashObject = "crash-object"

// lacksObject returns nil only when eng's World reports key not found; a
// failed lookup is an error, not an absence.
func lacksObject(ctx context.Context, eng world.Engine, key string) error {
	tx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	obj, found, err := tx.GetObject(ctx, key)
	world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	if found {
		return errors.New("object " + key + " survived its attempt's discard")
	}
	return nil
}

// runPluginOpApproval holds the plugin replay asks for awaiting the device's
// approval: the Space's health shows the wait for approval naming the plugin,
// and once the plugin is approved its handler registers and the waiting
// position applies on the same mount, with the wait cleared from the health.
func runPluginOpApproval(t *testing.T) {
	// Remount with replay waiting at the operation for approval.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	approve := make(chan struct{})
	s := newPluginOpSpace(ctx, t, approve)
	s.remountWaiting(ctx, t)
	defer func() { s.engRef.Release() }()
	accessor, ok := s.f.space(t).(sobject.SharedObjectHealthAccessor)
	if !ok {
		t.Fatal("the Space's shared object exposes no health")
	}
	health, releaseHealth, err := accessor.AccessSharedObjectHealth(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseHealth()

	// The health shows the wait for approval of the plugin.
	waitHealth(ctx, t, health, "awaiting approval", func(h *sobject.SharedObjectHealth) bool {
		return slices.Equal(h.GetReplayWait().GetApproval().GetPluginIds(), []string{testOpPlugin})
	})

	// Approving the plugin applies the waiting position without a remount.
	close(approve)
	if _, err := s.eng.WaitSeqno(ctx, s.seqno); err != nil {
		t.Fatal(err)
	}
	if err := world.AccessEngine(ctx, s.eng, func(ctx context.Context, eng world.Engine) error {
		return hasObjects(ctx, eng, "plugin-object")
	}); err != nil {
		t.Fatal(err)
	}
	waitHealth(ctx, t, health, "applied", func(h *sobject.SharedObjectHealth) bool {
		return h.GetReplayWait() == nil
	})
}

// waitHealth waits until health satisfies ok, failing with what when ctx
// ends first.
func waitHealth(ctx context.Context, t *testing.T, health ccontainer.Watchable[*sobject.SharedObjectHealth], what string, ok func(*sobject.SharedObjectHealth) bool) {
	t.Helper()
	cur := health.GetValue()
	for !ok(cur) {
		next, err := health.WaitValueChange(ctx, cur, nil)
		if err != nil {
			t.Fatalf("health never showed %s: last %s", what, cur.MarshalProtoText())
		}
		cur = next
	}
}

// runPluginOpDeadline makes the plugin's handler answer replay's first run of
// its operation with the error a plugin call returns at the operation
// deadline. The deadline is a fact about this device, not the operation, so
// replay must try the operation again rather than refuse it: the operation
// applies, and the World reaches the seqno it had before the remount.
func runPluginOpDeadline(t *testing.T) {
	// Remount with replay waiting at the operation, and fail the first
	// replayed run at the deadline.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	s := newPluginOpSpace(ctx, t, nil)
	s.marks.setTimeouts(1)
	s.remountWaiting(ctx, t)
	defer func() { s.engRef.Release() }()

	// Register the handler: the timed-out run stops replay, and the next
	// applies the operation.
	close(s.start)
	if _, err := s.eng.WaitSeqno(ctx, s.seqno); err != nil {
		t.Fatal(err)
	}
	if err := world.AccessEngine(ctx, s.eng, func(ctx context.Context, eng world.Engine) error {
		return hasObjects(ctx, eng, "plugin-object")
	}); err != nil {
		t.Fatal(err)
	}
	if n := s.marks.getTimeouts(); n != 0 {
		t.Fatalf("%d deadline errors left unreturned, want the replayed run to meet it", n)
	}
}

// runPluginOpWaitingWrite tries to make replay's wait for a handler lose a
// write: a transaction commits while replay waits, the Space is remounted
// before the handler registers, and only then the handler registers. The
// commit must not be acknowledged while the operation before it is undecided,
// and once ordered the write must survive the remount and apply after the
// handler arrives.
func runPluginOpWaitingWrite(t *testing.T) {
	// Remount with replay waiting at the operation.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	s := newPluginOpSpace(ctx, t, nil)
	s.remountWaiting(ctx, t)
	so := s.f.space(t)
	before := orderLen(ctx, t, so)

	// Commit a write on the waiting engine.
	committed := make(chan error, 1)
	eng := s.eng
	go func() {
		committed <- func() error {
			tx, err := eng.NewTransaction(ctx, true)
			if err != nil {
				return err
			}
			defer tx.Discard()
			obj, err := tx.CreateObject(ctx, "waiting-write", nil)
			world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
			return tx.Commit(ctx)
		}()
	}()

	// Wait until the write is in the order, unacknowledged.
	for orderLen(ctx, t, so) <= before {
		select {
		case err := <-committed:
			t.Fatalf("write ended before it was ordered: %v", err)
		case <-ctx.Done():
			t.Fatal("the write never reached the order")
		case <-time.After(10 * time.Millisecond):
		}
	}
	select {
	case err := <-committed:
		t.Fatalf("write ended while the operation before it waits for a handler: %v", err)
	default:
	}

	// Remount again before the handler registers. The write passed the queue,
	// so the writer learns its durability is unknown, not that it failed.
	s.eng, s.engRef = s.f.remount(ctx, t, s.eng, s.engRef)
	defer func() { s.engRef.Release() }()
	select {
	case err := <-committed:
		if !errors.Is(err, world.ErrDurabilityUnknown) {
			t.Fatalf("writer saw %v at the remount, want %v", err, world.ErrDurabilityUnknown)
		}
	case <-ctx.Done():
		t.Fatal("the writer never learned the remount ended its admission")
	}

	// Register the handler.
	close(s.start)

	// The write applies after the operation.
	if _, err := s.eng.WaitSeqno(ctx, s.seqno+1); err != nil {
		t.Fatal(err)
	}
	if err := world.AccessEngine(ctx, s.eng, func(ctx context.Context, eng world.Engine) error {
		return hasObjects(ctx, eng, "plugin-object", "waiting-write")
	}); err != nil {
		t.Fatal(err)
	}
}

// orderLen returns the number of operations above the checkpoint of so.
func orderLen(ctx context.Context, t *testing.T, so sobject.SharedObject) int {
	t.Helper()
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	order, err := sobject.ReadOrder(ctx, snap)
	if err != nil {
		t.Fatal(err)
	}
	return len(order.Order)
}

// listTestOpPlugin lists the test plugin in the Space settings of eng's World.
func listTestOpPlugin(ctx context.Context, t *testing.T, eng world.Engine) {
	t.Helper()
	err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		_, _, err := space_world_ops.SetSpaceSettings(ctx, ws, "", "", &space_world.SpaceSettings{PluginIds: []string{testOpPlugin}}, true, time.Now())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// startTestOpPlugin registers the plugin's handler for the mock operations
// until the returned release.
func startTestOpPlugin(ctx context.Context, t *testing.T, b bus.Bus, marks *replayMarks) func() {
	t.Helper()
	release, err := b.AddController(ctx, world.NewLookupOpController(testOpPlugin, "", marks.lookupOp), nil)
	if err != nil {
		t.Fatal(err)
	}
	return release
}

// testOpPluginLoader stands in for a Space resource's operation plugin
// loader: it reports the plugins the first LoadOpPlugins for its engine asks
// for, and starts the test plugin.
type testOpPluginLoader struct {
	// b is the bus the plugin registers on.
	b bus.Bus
	// engineID is the Space World engine.
	engineID string
	// loads receives the plugins of the first request.
	loads chan []string
	// start, when not nil, closes to start the plugin.
	start chan struct{}
	// approve, when not nil, holds the plugin awaiting approval, resolved as
	// a LoadOpPluginsValue as the Space's loader resolves the scheduler's
	// PluginSwapAwaitingApproval, until it closes.
	approve chan struct{}
	// marks records the replay mark of each run of the plugin's operations.
	marks *replayMarks
}

// GetControllerInfo returns information about the controller.
func (l *testOpPluginLoader) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/op-plugin-loader", controller.MustParseVersion("0.0.1"), "test operation plugin loader")
}

// Execute returns immediately.
func (l *testOpPluginLoader) Execute(context.Context) error {
	return nil
}

// HandleDirective starts the test plugin for the engine's first request.
func (l *testOpPluginLoader) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(space_world.LoadOpPlugins)
	if !ok || dir.LoadOpPluginsEngineID() != l.engineID {
		return nil, nil
	}
	select {
	case l.loads <- dir.LoadOpPluginsPluginIDs():
	default:
		return nil, nil
	}
	startPlugin := func() {
		_, _ = l.b.AddController(context.WithoutCancel(ctx), world.NewLookupOpController(testOpPlugin, "", l.marks.lookupOp), nil)
	}
	if l.approve == nil {
		go func() {
			if l.start != nil {
				<-l.start
			}
			startPlugin()
		}()
		return nil, nil
	}

	// Report the plugin awaiting approval, then start it once approved.
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, h directive.ResolverHandler) error {
		valID, _ := h.AddValue(space_world.LoadOpPluginsValue{testOpPlugin})
		select {
		case <-ctx.Done():
			return nil
		case <-l.approve:
		}
		h.RemoveValue(valID)
		startPlugin()
		<-ctx.Done()
		return nil
	}), nil)
}

// Close releases nothing.
func (l *testOpPluginLoader) Close() error {
	return nil
}

// replayMarks records space_world.IsReplay for each run of the mock
// operations it looks up.
type replayMarks struct {
	mtx   sync.Mutex
	marks []bool
	// timeouts is the number of replayed runs still to fail with the error a
	// plugin call returns at the operation deadline.
	timeouts int
	// missing counts the runs left that fail as a build missing on this
	// device, before recording a mark.
	missing atomic.Int32
	// crashes counts the runs left, after the missing ones, that write
	// crashObject and then fail as a handler lost after it was sent.
	crashes atomic.Int32
}

// setTimeouts fails the next n replayed runs at the operation deadline.
func (m *replayMarks) setTimeouts(n int) {
	m.mtx.Lock()
	m.timeouts = n
	m.mtx.Unlock()
}

// getTimeouts returns the replayed runs still to fail at the deadline.
func (m *replayMarks) getTimeouts() int {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.timeouts
}

// lookupOp looks up a mock operation that records its runs' replay marks.
func (m *replayMarks) lookupOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	op, err := world_mock.LookupMockOp(ctx, opTypeID)
	if op == nil || err != nil {
		return op, err
	}
	return &markedOp{Operation: op, marks: m}, nil
}

// get returns the marks recorded so far.
func (m *replayMarks) get() []bool {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return slices.Clone(m.marks)
}

// markedOp records the replay mark of each run, then runs the operation, or
// fails a replayed run at the operation deadline while marks has timeouts left.
// Before that it fails the runs marks counts as missing builds or lost
// handlers.
type markedOp struct {
	world.Operation
	marks *replayMarks
}

// ApplyWorldOp records whether ctx carries the replay mark and applies the
// operation.
func (o *markedOp) ApplyWorldOp(ctx context.Context, le *logrus.Entry, ws world.WorldState, sender peer.ID) (bool, error) {
	// Fail as a missing build, or write and fail as a lost handler.
	if n := o.marks.missing.Load(); n > 0 && o.marks.missing.CompareAndSwap(n, n-1) {
		return false, &world.PluginExecutableMissingError{PluginID: testOpPlugin, OperationTypeID: o.TypeID(), ManifestRoot: "root"}
	}
	if n := o.marks.crashes.Load(); n > 0 && o.marks.crashes.CompareAndSwap(n, n-1) {
		obj, err := ws.CreateObject(ctx, crashObject, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			return false, err
		}
		return false, &world.PluginUnavailableError{PluginID: testOpPlugin, OperationTypeID: o.TypeID(), Sent: true, Err: errors.New("connection lost")}
	}

	// Record the mark, failing a replayed run at the deadline.
	replay := space_world.IsReplay(ctx)
	o.marks.mtx.Lock()
	o.marks.marks = append(o.marks.marks, replay)
	timeout := replay && o.marks.timeouts != 0
	if timeout {
		o.marks.timeouts--
	}
	o.marks.mtx.Unlock()
	if timeout {
		return false, &world.PluginOpTimeoutError{PluginID: testOpPlugin, OperationTypeID: o.TypeID(), Deadline: world.WriteBudgetLength}
	}
	return o.Operation.ApplyWorldOp(ctx, le, ws, sender)
}
