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
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// testOpPlugin is the plugin the World declares handles the mock World
// operation.
const testOpPlugin = "test-plugin"

// runPluginOpThrough writes an operation whose type only a plugin resolves,
// then remounts without the plugin: replay stops at the operation, asks for
// the plugin the World declares with LoadOpPlugins, and continues once the
// plugin's handler registers. The author's run of the operation carries no
// replay mark, and replay's run carries it. A pinned build missing on this
// device fails the live write and enqueues nothing, and stops replay at the
// operation without a stored refusal. A handler that writes part of its
// effects and then fails after it was sent also stops replay at the operation,
// and the retry applies on the World as it was before that attempt, so the
// result equals one clean apply.
func runPluginOpThrough(t *testing.T) {
	// Resolve the mock World operation only through the plugin, which the
	// World declares.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	f := newAdapterFixtureOn(t, fixtureOpts{opTypes: &space_world.OpTypes{
		Declared: func(_ context.Context, _ world.WorldState, opTypeID string) ([]string, error) {
			if opTypeID == world_mock.MockWorldOpTypeID {
				return []string{testOpPlugin}, nil
			}
			return nil, nil
		},
	}})

	// Fail a write on a build missing on this device, while the plugin runs.
	marks := &replayMarks{}
	releasePlugin := startTestOpPlugin(ctx, t, f.tb.Bus, marks)
	eng, engRef := f.start(t)
	so := f.space(t)
	before := opCount(ctx, t, so)
	write := func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, "plugin-object", nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		op := &markedOp{Operation: world_mock.NewMockWorldOp("plugin-object", "hello"), marks: marks}
		_, _, err = ws.ApplyWorldOp(ctx, op, "")
		return err
	}
	marks.missing.Store(1)
	var missing *world.PluginExecutableMissingError
	if err := world.ExecTransaction(ctx, eng, true, write); !errors.As(err, &missing) {
		t.Fatalf("write on a missing build = %v, want PluginExecutableMissingError", err)
	}

	// Write the operation. The failed write left no record before it, so the
	// order grows by this write's operation alone.
	if err := world.ExecTransaction(ctx, eng, true, write); err != nil {
		t.Fatal(err)
	}
	waitOps(ctx, t, so, before+1)
	if n := opCount(ctx, t, so); n != before+1 {
		t.Fatalf("operations = %d, want %d: the failed write left a record", n, before+1)
	}
	if got := marks.get(); len(got) == 0 || got[0] {
		t.Fatalf("author's run replay marks = %v, want the first unmarked", got)
	}
	authored := len(marks.get())
	var authoredSeqno uint64
	if err := world.AccessEngine(ctx, eng, func(ctx context.Context, eng world.Engine) error {
		var err error
		authoredSeqno, err = eng.GetSeqno(ctx)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Remount without the plugin, with a loader that starts it on request.
	// Its first run on replay meets the missing build and waits as a gap; its
	// second writes an object and then fails as sent, and waits as a gap too.
	releasePlugin()
	marks.missing.Store(1)
	marks.crashes.Store(1)
	loads := make(chan []string, 1)
	releaseLoader, err := f.tb.Bus.AddController(ctx, &testOpPluginLoader{b: f.tb.Bus, engineID: f.engineID, loads: loads, marks: marks}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseLoader()
	type mount struct {
		eng *world.BusEngine
		ref directive.Reference
	}
	remounted := make(chan mount, 1)
	go func() {
		eng, ref := f.remount(ctx, t, eng, engRef)
		remounted <- mount{eng: eng, ref: ref}
	}()

	// Replay asks for the plugin and reaches the World after the operation.
	select {
	case ids := <-loads:
		if !slices.Equal(ids, []string{testOpPlugin}) {
			t.Fatalf("replay asked for plugins %q, want %s", ids, testOpPlugin)
		}
	case <-ctx.Done():
		t.Fatal("replay stopped at the plugin's operation without asking for the plugin")
	}
	m := <-remounted
	defer m.ref.Release()
	// The retry applied on the World before the failed attempt: its object is
	// gone and the seqno is the author's, as after one clean apply.
	if err := world.AccessEngine(ctx, m.eng, func(ctx context.Context, eng world.Engine) error {
		if err := hasObjects(ctx, eng, "plugin-object"); err != nil {
			return err
		}
		if err := hasObjects(ctx, eng, crashObject); err == nil {
			return errors.New("the failed attempt's object survived its discard")
		}
		seqno, err := eng.GetSeqno(ctx)
		if err == nil && seqno != authoredSeqno {
			err = errors.New("replayed seqno differs from one clean apply")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := marks.get()[authored:]; len(got) == 0 || slices.Contains(got, false) {
		t.Fatalf("replay's runs replay marks = %v, want every one marked", got)
	}
	if n := marks.missing.Load(); n != 0 {
		t.Fatalf("replay never met the missing build: %d left", n)
	}
	if n := marks.crashes.Load(); n != 0 {
		t.Fatalf("replay never met the failing handler: %d left", n)
	}
}

// crashObject is the object a failing run of the plugin's handler writes
// before it fails.
const crashObject = "crash-object"

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
		go func() {
			_, _ = l.b.AddController(context.WithoutCancel(ctx), world.NewLookupOpController(testOpPlugin, "", l.marks.lookupOp), nil)
		}()
	default:
	}
	return nil, nil
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
	// missing counts the runs left that fail as a build missing on this
	// device, before recording a mark.
	missing atomic.Int32
	// crashes counts the runs left, after the missing ones, that write
	// crashObject and then fail as a handler lost after it was sent.
	crashes atomic.Int32
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

// markedOp records the replay mark of each run, then runs the operation.
type markedOp struct {
	world.Operation
	marks *replayMarks
}

// ApplyWorldOp records whether ctx carries the replay mark and applies the
// operation.
func (o *markedOp) ApplyWorldOp(ctx context.Context, le *logrus.Entry, ws world.WorldState, sender peer.ID) (bool, error) {
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
	o.marks.mtx.Lock()
	o.marks.marks = append(o.marks.marks, space_world.IsReplay(ctx))
	o.marks.mtx.Unlock()
	return o.Operation.ApplyWorldOp(ctx, le, ws, sender)
}
