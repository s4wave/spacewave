// Package core_alarm_watch runs Alarm watches: each keeps one Alarm in step
// with a PureFunction's view of PromQL queries and World objects.
package core_alarm_watch

import (
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_alarm "github.com/s4wave/spacewave/sdk/alarm"
	alarm_watch "github.com/s4wave/spacewave/sdk/alarm/watch"
	s4wave_function "github.com/s4wave/spacewave/sdk/function"
	function_run "github.com/s4wave/spacewave/sdk/function/run"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "alarm/watch"

// retryDelay is how long the watch waits before reopening a failed source.
const retryDelay = 5 * time.Second

// Controller keeps one Alarm in step with its function. It evaluates after
// each change to the source's targets, to the function or to a read object,
// and when a hold time ends, and writes the Alarm only when it differs.
type Controller struct {
	// le is the logger.
	le *logrus.Entry
	// b resolves the World engine and the source's object type.
	b bus.Bus
	// conf is the config.
	conf *alarm_watch.Config
	// runner runs the function.
	runner *function_run.Runner
	// now returns the evaluation time.
	now func() time.Time
	// engine is the World engine holding the Alarm.
	engine *world.BusEngine
	// linked is set once the Alarm's reads edges match reads.
	linked bool
	// reads are the keys the Alarm's reads edges point at.
	reads []string
}

// NewController constructs an Alarm watch controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *alarm_watch.Config, runner *function_run.Runner) *Controller {
	return &Controller{
		le:     le,
		b:      b,
		conf:   conf,
		runner: runner,
		now:    time.Now,
		engine: world.NewBusEngine(nil, b, conf.GetEngineId()),
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "alarm watch controller")
}

// Execute evaluates the watch until the context ends.
func (c *Controller) Execute(ctx context.Context) error {
	// Bind the World engine to the controller context.
	c.engine.SetContext(ctx)
	ws := world.NewEngineWorldState(c.engine, true)
	src := newSource(c)
	defer src.release()

	// Evaluate, then wait for an input to change or a hold time to end.
	for {
		reads, holdEnds, err := c.evaluate(ctx, src)
		if err != nil {
			return err
		}
		if err := c.wait(ctx, ws, src, reads, holdEnds); err != nil {
			return err
		}
	}
}

// evaluate runs the function on the current inputs and stores the Alarm it
// implies. It returns what the evaluation read from the World and when the
// Alarm's hold time ends, if it is holding. A failure to read an input or run
// the function shows on the Alarm; only a World failure or the end of ctx
// returns an error.
func (c *Controller) evaluate(ctx context.Context, src *source) (*worldReads, time.Time, error) {
	// Read the function and the objects at one point in the World.
	now := c.now()
	in, fn, reads, readErr := c.readWorld(ctx)
	if ctx.Err() != nil {
		return nil, time.Time{}, ctx.Err()
	}
	if reads == nil {
		return nil, time.Time{}, readErr
	}

	// Query the source, then run the function.
	var result alarm_watch.WatchResult
	evalErr := readErr
	if evalErr == nil {
		evalErr = c.query(ctx, src, in, now)
	}
	if evalErr == nil {
		evalErr = c.runner.Run(ctx, fn, in, &result)
	}
	if ctx.Err() != nil {
		return nil, time.Time{}, ctx.Err()
	}

	// Store the Alarm the result implies, or the failure.
	cond := &s4wave_alarm.Condition{Firing: result.GetFiring(), Summary: result.GetSummary(), Options: result.GetOptions()}
	next, err := c.store(ctx, cond, evalErr, c.existingReads(reads.seen), now)
	if err != nil {
		return nil, time.Time{}, err
	}
	if evalErr != nil {
		c.le.WithError(evalErr).Debug("alarm watch evaluation failed")
	}
	holdEnds, _ := next.HoldEnds()
	return reads, holdEnds, nil
}

// worldReads is what one evaluation read from the World.
type worldReads struct {
	// seqno is the World seqno the reads observed.
	seqno uint64
	// set holds the reads, so a change to an object or to an object's type
	// edge ends the wait. A type change does not revise the object.
	set *world.ReadSet
	// seen holds the root of the function, each object and the source, in
	// that order.
	seen []*world.ObjectRootRef
}

// readWorld reads the function and the objects it reads, and returns the
// function's input with the objects filled in, with the root of each object
// read. It returns no roots when the World cannot be read.
func (c *Controller) readWorld(ctx context.Context) (*alarm_watch.WatchInput, *s4wave_function.PureFunction, *worldReads, error) {
	// Open a read transaction.
	tx, err := c.engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, nil, nil, err
	}
	defer tx.Discard()
	seqno, err := tx.GetSeqno(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	set := world.NewReadSet()
	rtx := set.Track(tx)

	// Read the roots of the function, the source and each object, so a change
	// to any of them wakes the watch.
	keys := append([]string{c.conf.GetFunctionKey()}, c.conf.GetObjectKeys()...)
	if c.conf.GetSourceKey() != "" {
		keys = append(keys, c.conf.GetSourceKey())
	}
	seen, err := world.GetObjectRootRefsBatch(ctx, rtx, keys)
	if err != nil {
		return nil, nil, nil, err
	}
	reads := &worldReads{seqno: seqno, set: set, seen: seen}

	// Describe each object to the function.
	in := &alarm_watch.WatchInput{}
	for i, key := range c.conf.GetObjectKeys() {
		ref := seen[1+i]
		obj := &alarm_watch.ObjectInput{Key: key, Exists: ref.Exists, Rev: ref.Rev}
		if ref.Exists {
			if obj.TypeId, err = world_types.GetObjectType(ctx, rtx, key); err != nil {
				return nil, nil, reads, err
			}
		}
		if in.Objects == nil {
			in.Objects = make(map[string]*alarm_watch.ObjectInput)
		}
		in.Objects[key] = obj
	}

	// Read the function.
	if !seen[0].Exists {
		return nil, nil, reads, errors.Errorf("function %q does not exist", c.conf.GetFunctionKey())
	}
	fn, err := s4wave_function.ReadPureFunction(ctx, rtx, c.conf.GetFunctionKey())
	if err != nil {
		return nil, nil, reads, err
	}
	return in, fn, reads, nil
}

// existingReads returns the sorted keys of the source and the read objects
// that exist in seen, which readWorld returned.
func (c *Controller) existingReads(seen []*world.ObjectRootRef) []string {
	var reads []string
	for _, ref := range seen[1:] {
		if ref.Exists {
			reads = append(reads, ref.ObjectKey)
		}
	}
	slices.Sort(reads)
	return slices.Compact(reads)
}

// query fills in the result of each query at now.
func (c *Controller) query(ctx context.Context, src *source, in *alarm_watch.WatchInput, now time.Time) error {
	// Open the source when the watch has queries.
	in.TimeMs = now.UnixMilli()
	if len(c.conf.GetQueries()) == 0 {
		return nil
	}
	client, err := src.client(ctx)
	if err != nil {
		return err
	}

	// Run each query at the evaluation time.
	in.Queries = make(map[string]*s4wave_prom.QueryResponse, len(c.conf.GetQueries()))
	for _, q := range c.conf.GetQueries() {
		resp, err := client.Query(ctx, &s4wave_prom.QueryRequest{Expr: q.GetExpr(), TimeMs: in.TimeMs})
		if err != nil {
			src.fail()
			return errors.Wrapf(err, "query %q", q.GetName())
		}
		in.Queries[q.GetName()] = resp
	}
	return nil
}

// store writes the Alarm that cond implies at now, or records evalErr on the
// current Alarm, and returns the Alarm. It points the Alarm's reads edges at
// reads. It writes nothing when neither would change. The read and the write
// share a transaction, so an acknowledgement written meanwhile is kept.
func (c *Controller) store(ctx context.Context, cond *s4wave_alarm.Condition, evalErr error, reads []string, now time.Time) (*s4wave_alarm.Alarm, error) {
	// Relink when the edges were never written or reads changed, then read,
	// advance and write the Alarm in one transaction.
	relink := !c.linked || !slices.Equal(reads, c.reads)
	var next *s4wave_alarm.Alarm
	err := world.ExecTransaction(ctx, c.engine, true, func(ctx context.Context, tx world.WorldState) error {
		// Read the current Alarm.
		cur, err := s4wave_alarm.ReadAlarm(ctx, tx, c.conf.GetAlarmKey())
		if err != nil {
			return err
		}

		// Advance it, or keep its state and show the failure.
		if evalErr != nil {
			next = cur.CloneVT()
			if next == nil {
				next = &s4wave_alarm.Alarm{}
			}
			next.Error = evalErr.Error()
		} else {
			next = cur.Next(cond, time.Duration(c.conf.GetHoldMs())*time.Millisecond, now)
		}
		next.Title = c.conf.GetTitle()

		// Write it only when it differs.
		if cur == nil || !next.EqualVT(cur) {
			if err := s4wave_alarm.WriteAlarm(ctx, tx, c.conf.GetAlarmKey(), next); err != nil {
				return err
			}
		}

		// Point its reads edges at what the watch reads.
		if !relink {
			return nil
		}
		return s4wave_alarm.SetReads(ctx, tx, c.conf.GetAlarmKey(), reads)
	})
	if err == nil && relink {
		c.linked, c.reads = true, reads
	}
	return next, err
}

// wait returns when a change touches reads, the source's targets change or
// its stream fails, or holdEnds passes, and returns the context's error when
// ctx ends.
func (c *Controller) wait(ctx context.Context, ws world.WorldState, src *source, reads *worldReads, holdEnds time.Time) error {
	// End the World wait on the first other wake.
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		var hold <-chan time.Time
		if !holdEnds.IsZero() {
			timer := time.NewTimer(time.Until(holdEnds))
			defer timer.Stop()
			hold = timer.C
		}
		select {
		case <-src.changed:
		case <-hold:
		case <-waitCtx.Done():
		}
		cancel()
	}()

	// Wait for a change since the reads to what they read. Starting from the
	// reads' seqno also catches an object deleted and created again since.
	_, err := reads.set.WaitChange(waitCtx, ws, reads.seqno)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil && waitCtx.Err() == nil {
		return err
	}
	return nil
}

// HandleDirective handles no directives.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
