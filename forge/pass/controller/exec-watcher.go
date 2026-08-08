package pass_controller

import (
	"context"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	"github.com/sirupsen/logrus"
)

// execWatcherKey identifies the execution snapshot a watcher is validating.
type execWatcherKey struct {
	objectKey      string
	executionState forge_execution.State
	peerID         string
	timestampSecs  int64
	timestampNanos int32
}

// execWatcher watches a Execution instance for completion.
type execWatcher struct {
	// c is the controller
	c *Controller
	// key is the execution snapshot this watcher validates.
	key execWatcherKey
}

func newExecWatcherKey(execState *forge_pass.ExecState) execWatcherKey {
	timestampSecs, timestampNanos := splitExecTimestamp(execState.GetTimestamp())
	return execWatcherKey{
		objectKey:      execState.GetObjectKey(),
		executionState: execState.GetExecutionState(),
		peerID:         execState.GetPeerId(),
		timestampSecs:  timestampSecs,
		timestampNanos: timestampNanos,
	}
}

func splitExecTimestamp(ts *timestamppb.Timestamp) (int64, int32) {
	if ts == nil {
		return 0, 0
	}
	return ts.Seconds, ts.Nanos
}

func (k execWatcherKey) String() string {
	return k.objectKey
}

func (k execWatcherKey) matchesExecution(exec *forge_execution.Execution) bool {
	timestampSecs, timestampNanos := splitExecTimestamp(exec.GetTimestamp())
	switch {
	case k.executionState != exec.GetExecutionState():
	case k.peerID != exec.GetPeerId():
	case k.timestampSecs != timestampSecs:
	case k.timestampNanos != timestampNanos:
	default:
		return true
	}
	return false
}

// newExecWatcher constructs an execution instance watcher.
func (c *Controller) newExecWatcher(key execWatcherKey) (keyed.Routine, *execWatcher) {
	exc := &execWatcher{
		c:   c,
		key: key,
	}
	return exc.execute, exc
}

// execute executes the Execution watcher.
func (e *execWatcher) execute(ctx context.Context) error {
	execObjKey := e.key.objectKey
	e.c.le.
		WithField("exec-object-key", execObjKey).
		WithField("exec-state", e.key.executionState.String()).
		Debug("watching execution object for changes")
	loop, _, ws := world_control.NewBusWatchLoop(
		ctx,
		e.c.le,
		e.c.bus,
		e.c.conf.GetEngineId(),
		false,
		execObjKey,
		e.processState,
	)
	return loop.Execute(ctx, ws)
}

// processState implements the state watcher loop.
func (e *execWatcher) processState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	if obj == nil {
		le.Debug("object does not exist")
		return true, nil
	}

	// unmarshal Execution state + build read cursor
	var exState *forge_execution.Execution
	_, err = world.AccessObject(ctx, ws.AccessWorldState, rootRef, func(bcs *block.Cursor) error {
		var berr error
		exState, berr = forge_execution.UnmarshalExecution(ctx, bcs)
		return berr
	})
	if err != nil {
		return false, err
	}

	// check if the execution state matches the ExecState
	if e.key.matchesExecution(exState) {
		// matches, continue to watch
		return true, nil
	}

	// does not match: stop this watcher & notify controller
	e.c.triggerSyncExecStates()
	return false, nil
}

// _ is a type assertion
var _ world_control.WatchLoopHandler = (*execWatcher)(nil).processState
