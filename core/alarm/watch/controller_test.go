package core_alarm_watch_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	core_alarm_watch "github.com/s4wave/spacewave/core/alarm/watch"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_alarm "github.com/s4wave/spacewave/sdk/alarm"
	alarm_watch "github.com/s4wave/spacewave/sdk/alarm/watch"
	s4wave_function "github.com/s4wave/spacewave/sdk/function"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

const (
	// alarmKey is the Alarm the watch writes.
	alarmKey = "alarms/forge-down"
	// functionKey is the function that decides the Alarm.
	functionKey = "functions/forge-down"
	// sourceKey is the object whose resource answers the queries.
	sourceKey = "prom/forge"
	// sourceType is the object type of sourceKey.
	sourceType = "test/prom"
	// hold is the watch's hold time.
	hold = 300 * time.Millisecond
)

// downSource fires while any instance's up sample is 0.
const downSource = `
interface Input {
  queries?: Record<string, { series?: { samples?: { value?: number }[] }[] }>
}

export default function forgeDown(input: Input) {
  const down = (input.queries?.up?.series ?? []).filter(
    (s) => ((s.samples ?? []).at(-1)?.value ?? 0) === 0,
  )
  if (down.length === 0) {
    return { summary: 'every instance is up' }
  }
  return {
    firing: true,
    summary: down.length + ' instance down',
    options: [{ label: 'Restart node_exporter', recommended: true }],
  }
}
`

// TestWatch walks an Alarm through clear, holding, firing, acknowledged and
// clear as the up query changes, and checks that the watch writes nothing
// while its result holds.
func TestWatch(t *testing.T) {
	// Start a World with the source and the function, and the watch.
	e := newEnv(t)
	e.prom.up.Store(1)
	e.createFunction(downSource)
	e.startWatch()

	// With every instance up the watch creates a clear Alarm.
	a := e.waitAlarm("clear", func(a *s4wave_alarm.Alarm) bool {
		return a.GetSummary() == "every instance is up"
	})
	if a.GetState() != s4wave_alarm.AlarmState_ALARM_STATE_CLEAR || a.GetTitle() != "Forge down" || a.GetError() != "" {
		t.Fatalf("first alarm: %v", a)
	}
	found, err := e.tb.WorldState.LookupGraphQuads(e.ctx, world.NewGraphQuadWithKeys(alarmKey, s4wave_alarm.ReadsPredicate, sourceKey, ""), 0)
	if err != nil || len(found) != 1 {
		t.Fatalf("alarm reads edge to the source: %v %v", found, err)
	}

	// An instance going down holds the Alarm, then fires it once the hold
	// time passes with no further commit.
	e.prom.up.Store(0)
	e.prom.commit()
	a = e.waitAlarm("holding", func(a *s4wave_alarm.Alarm) bool {
		return a.GetState() == s4wave_alarm.AlarmState_ALARM_STATE_HOLDING
	})
	if a.GetSummary() != "1 instance down" || len(a.GetOptions()) != 1 {
		t.Fatalf("holding alarm: %v", a)
	}
	a = e.waitAlarm("firing", func(a *s4wave_alarm.Alarm) bool {
		return a.GetState() == s4wave_alarm.AlarmState_ALARM_STATE_FIRING
	})
	if a.GetFiredMs()-a.GetSinceMs() < hold.Milliseconds() {
		t.Fatalf("alarm fired before its hold time: %v", a)
	}

	// Acknowledge it, and check that evaluations with the same result keep
	// the acknowledgement and write nothing.
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		return s4wave_alarm.AcknowledgeAlarm(ctx, tx, alarmKey, "cjs", time.Now())
	})
	rev := e.alarmRev()
	e.evaluations(3)
	if got := e.alarmRev(); got != rev {
		t.Fatalf("alarm revision moved from %d to %d with no change", rev, got)
	}
	if a := e.readAlarm(); a.GetState() != s4wave_alarm.AlarmState_ALARM_STATE_ACKNOWLEDGED || a.GetAcknowledgedBy() != "cjs" {
		t.Fatalf("acknowledged alarm: %v", a)
	}

	// The instance coming back clears the Alarm by itself.
	e.prom.up.Store(1)
	e.prom.commit()
	a = e.waitAlarm("cleared", func(a *s4wave_alarm.Alarm) bool {
		return a.GetState() == s4wave_alarm.AlarmState_ALARM_STATE_CLEAR
	})
	if a.GetClearedMs() == 0 || a.GetAcknowledgedBy() != "" || a.GetSummary() != "every instance is up" {
		t.Fatalf("cleared alarm: %v", a)
	}
}

// TestWatchFunctionError shows a failing function on the Alarm, keeps the
// Alarm's state, and clears the error once the function is fixed.
func TestWatchFunctionError(t *testing.T) {
	// Start with a function that throws.
	e := newEnv(t)
	e.prom.up.Store(1)
	e.createFunction(`export default function broken() { throw new Error('no answer') }`)
	e.startWatch()

	// The failure shows on the Alarm, which stays clear.
	a := e.waitAlarm("error", func(a *s4wave_alarm.Alarm) bool {
		return a.GetError() != ""
	})
	if a.GetState() != s4wave_alarm.AlarmState_ALARM_STATE_CLEAR {
		t.Fatalf("failed alarm: %v", a)
	}

	// Replacing the function wakes the watch and clears the error.
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		_, err := tx.DeleteObject(ctx, functionKey)
		return err
	})
	e.createFunction(downSource)
	e.waitAlarm("fixed", func(a *s4wave_alarm.Alarm) bool {
		return a.GetError() == "" && a.GetSummary() == "every instance is up"
	})
}

// env is a World with a prom source and a watch controller.
type env struct {
	// t is the test.
	t *testing.T
	// ctx is the test context.
	ctx context.Context
	// tb is the World testbed.
	tb *world_testbed.Testbed
	// prom serves the source's queries.
	prom *fakeProm
}

// newEnv starts a World with the watch factory and the source object type,
// and creates the source object.
func newEnv(t *testing.T) *env {
	// Start the World and register the watch factory and the source type.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	tb := world_testbed.MustDefault(t, ctx)
	tb.StaticResolver.AddFactory(core_alarm_watch.NewFactory(tb.Bus))
	e := &env{t: t, ctx: ctx, tb: tb, prom: &fakeProm{}}
	release, err := tb.Bus.AddController(ctx, &promTypes{prom: e.prom}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	// Create the source object.
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		obj, _, err := world.CreateWorldObject(ctx, tx, sourceKey, func(bcs *block.Cursor) error {
			bcs.SetBlock(&s4wave_prom.Dashboard{}, true)
			return nil
		})
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		return world_types.SetObjectType(ctx, tx, sourceKey, sourceType)
	})
	return e
}

// exec runs fn in a write transaction.
func (e *env) exec(fn func(ctx context.Context, tx world.WorldState) error) {
	e.t.Helper()
	if err := world.ExecTransaction(e.ctx, e.tb.Engine, true, fn); err != nil {
		e.t.Fatal(err)
	}
}

// createFunction compiles source and creates it as the watch's function.
func (e *env) createFunction(source string) {
	// Compile the source.
	e.t.Helper()
	module, err := s4wave_function.Compile(source)
	if err != nil {
		e.t.Fatal(err)
	}

	// Store it as a function/pure object.
	fn := &s4wave_function.PureFunction{
		Source:     source,
		Module:     module,
		InputType:  "s4wave.alarm.watch.WatchInput",
		OutputType: "s4wave.alarm.watch.WatchResult",
	}
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		return s4wave_function.CreatePureFunction(ctx, tx, functionKey, fn)
	})
}

// startWatch starts the watch controller.
func (e *env) startWatch() {
	// Load the controller with the watch's config and wait for it to run.
	e.t.Helper()
	conf := &alarm_watch.Config{
		EngineId:    e.tb.EngineID,
		AlarmKey:    alarmKey,
		FunctionKey: functionKey,
		HoldMs:      hold.Milliseconds(),
		Title:       "Forge down",
		SourceKey:   sourceKey,
		Queries:     []*alarm_watch.Query{{Name: "up", Expr: "up"}},
	}
	_, _, ref, err := loader.WaitExecControllerRunning(e.ctx, e.tb.Bus, resolver.NewLoadControllerWithConfig(conf), nil)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(ref.Release)
}

// readAlarm reads the Alarm.
func (e *env) readAlarm() *s4wave_alarm.Alarm {
	e.t.Helper()
	a, err := s4wave_alarm.ReadAlarm(e.ctx, e.tb.WorldState, alarmKey)
	if err != nil {
		e.t.Fatal(err)
	}
	return a
}

// alarmRev returns the revision of the Alarm's object.
func (e *env) alarmRev() uint64 {
	e.t.Helper()
	refs, err := world.GetObjectRootRefsBatch(e.ctx, e.tb.WorldState, []string{alarmKey})
	if err != nil {
		e.t.Fatal(err)
	}
	return refs[0].Rev
}

// waitAlarm waits until the Alarm exists and done accepts it, and returns it.
func (e *env) waitAlarm(what string, done func(a *s4wave_alarm.Alarm) bool) *s4wave_alarm.Alarm {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(e.ctx, 10*time.Second)
	defer cancel()
	for {
		// Read the Alarm at the current sequence number.
		seqno, err := e.tb.WorldState.GetSeqno(ctx)
		if err != nil {
			e.t.Fatal(err)
		}
		if a := e.readAlarm(); a != nil && done(a) {
			return a
		}

		// Wait for the next commit.
		if _, err := e.tb.WorldState.WaitSeqno(ctx, seqno+1); err != nil {
			e.t.Fatalf("waiting for the %s alarm: %v, last %v", what, err, e.readAlarm())
		}
	}
}

// evaluations commits n target changes and waits until the watch has
// evaluated each of them.
func (e *env) evaluations(n int) {
	// Commit one more change than asked, since the watch queries before it
	// stores, so the last query shows the evaluation before it finished.
	e.t.Helper()
	for range n + 1 {
		e.evaluation()
	}
}

// evaluation commits a target change and waits for the watch to query it.
func (e *env) evaluation() {
	// Commit the change.
	e.t.Helper()
	ctx, cancel := context.WithTimeout(e.ctx, 10*time.Second)
	defer cancel()
	start := e.prom.queries.Load()
	e.prom.commit()

	// Wait for the watch's next query.
	for e.prom.queries.Load() == start {
		if ctx.Err() != nil {
			e.t.Fatal("the watch did not evaluate the target change")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// fakeProm answers the up query with one instance whose value is up, and
// sends a targets change on each commit.
type fakeProm struct {
	// up is the instance's up value.
	up atomic.Int64
	// queries counts the queries answered.
	queries atomic.Int32

	// mu guards watchers.
	mu sync.Mutex
	// watchers wakes each open targets stream.
	watchers []chan struct{}
}

// commit wakes every open targets stream.
func (f *fakeProm) commit() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ch := range f.watchers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Query answers any expression with the instance's up sample.
func (f *fakeProm) Query(_ context.Context, req *s4wave_prom.QueryRequest) (*s4wave_prom.QueryResponse, error) {
	defer f.queries.Add(1)
	return &s4wave_prom.QueryResponse{
		ResultType: s4wave_prom.ResultType_RESULT_TYPE_VECTOR,
		Series: []*s4wave_prom.Series{{
			Labels:  &s4wave_prom.LabelSet{Labels: []*s4wave_prom.Label{{Name: "instance", Value: "forge:9100"}}},
			Samples: []*s4wave_prom.Sample{{TimeMs: req.GetTimeMs(), Value: float64(f.up.Load())}},
		}},
	}, nil
}

// LabelNames returns no names.
func (f *fakeProm) LabelNames(context.Context, *s4wave_prom.LabelNamesRequest) (*s4wave_prom.LabelsResponse, error) {
	return &s4wave_prom.LabelsResponse{}, nil
}

// LabelValues returns no values.
func (f *fakeProm) LabelValues(context.Context, *s4wave_prom.LabelValuesRequest) (*s4wave_prom.LabelsResponse, error) {
	return &s4wave_prom.LabelsResponse{}, nil
}

// WatchTargets sends a change after each commit until the stream ends.
func (f *fakeProm) WatchTargets(_ *s4wave_prom.WatchTargetsRequest, stream s4wave_prom.SRPCPromQueryResourceService_WatchTargetsStream) error {
	// Register the stream's wake.
	ch := make(chan struct{}, 1)
	f.mu.Lock()
	f.watchers = append(f.watchers, ch)
	f.mu.Unlock()

	// Send a change after each commit.
	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-ch:
			if err := stream.Send(&s4wave_prom.WatchTargetsResponse{}); err != nil {
				return err
			}
		}
	}
}

// promTypes registers sourceType on the bus the way a plugin does.
type promTypes struct {
	// prom serves every object of the type.
	prom *fakeProm
}

// GetControllerInfo returns information about the controller.
func (c *promTypes) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/prom-types", controller.MustParseVersion("0.0.1"), "prom source object type")
}

// Execute returns at once, since the controller only handles directives.
func (c *promTypes) Execute(context.Context) error {
	return nil
}

// HandleDirective answers the lookup of sourceType.
func (c *promTypes) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(objecttype.LookupObjectType)
	if !ok || dir.LookupObjectTypeID() != sourceType {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]objecttype.LookupObjectTypeValue{objecttype.NewObjectType(sourceType, c.newResource)}), nil)
}

// newResource serves the fake's PromQueryResourceService.
func (c *promTypes) newResource(context.Context, *logrus.Entry, bus.Bus, world.Engine, world.WorldState, string) (srpc.Invoker, func(), error) {
	mux := srpc.NewMux()
	if err := s4wave_prom.SRPCRegisterPromQueryResourceService(mux, c.prom); err != nil {
		return nil, nil, err
	}
	return mux, func() {}, nil
}

// Close releases nothing.
func (c *promTypes) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*promTypes)(nil)
