package resource_worldop_registry

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// handlerWriteGuard guards the World handle one plugin op handler call writes
// through, and keeps the first refusal. A handler that catches the refusal
// and returns success still fails its op, so a refused op leaves none of its
// writes: the transaction applying it discards them.
//
// Only an *world.OperationRejection is a refusal. Any other error from the
// wrapped guard, such as a failed read of an object's type, is a failure to
// decide: the guard keeps the first one and the op fails as a system error,
// so the author's run retries it instead of rejecting it for good.
//
// A guard in record mode, for an op the Space replays from its log, refuses
// nothing: the op's author admitted it, so refusing it now would build a World
// the author never had. It logs and counts each write the guard would refuse.
type handlerWriteGuard struct {
	// guard decides each write.
	guard world.WriteGuard
	// record lets every write through, logging each one guard refuses.
	record bool
	// le logs recorded writes, with only their op type and keys.
	le *logrus.Entry

	// mtx guards refused, failed and recorded.
	mtx sync.Mutex
	// refused is the first refusal guard returned during the call.
	refused error
	// failed is the first error guard returned that is not a refusal.
	failed error
	// recorded counts the writes record mode let through over a refusal.
	recorded int
}

// newHandlerWriteGuard wraps guard for one handler call of the op opTypeID,
// recording instead of refusing when record is set.
func newHandlerWriteGuard(le *logrus.Entry, guard world.WriteGuard, opTypeID string, record bool) *handlerWriteGuard {
	return &handlerWriteGuard{
		guard:  guard,
		record: record,
		le:     le.WithField("operation-type-id", opTypeID),
	}
}

// CheckWrite checks w with the wrapped guard. It keeps the first refusal and
// the first failure to decide, or in record mode logs either and allows the
// write.
func (g *handlerWriteGuard) CheckWrite(ctx context.Context, ws world.WorldState, w *world.GuardedWrite) error {
	// Allow what the wrapped guard allows.
	err := g.guard.CheckWrite(ctx, ws, w)
	if err == nil {
		return nil
	}

	// Record a refused write on replay and let it through.
	if g.record {
		g.mtx.Lock()
		g.recorded++
		g.mtx.Unlock()
		g.le.WithFields(recordedWriteFields(w)).Warn("replay applied a write the plugin guard refuses")
		return nil
	}

	// Keep the first refusal, or the first failure to decide.
	g.mtx.Lock()
	switch {
	case world.IsGuardSysErr(err):
		if g.failed == nil {
			g.failed = err
		}
	case g.refused == nil:
		g.refused = err
	}
	g.mtx.Unlock()
	return err
}

// recordedWriteFields returns the log fields of a recorded write: the type of
// the op the write applies and the object keys the write references, and
// nothing of the write's content.
// A quad contributes the keys of its subject and object, never a literal.
func recordedWriteFields(w *world.GuardedWrite) logrus.Fields {
	keys := w.ObjectKeys
	if q := w.Quad; q != nil {
		for _, value := range []string{q.GetSubject(), q.GetObj()} {
			if key, err := world.GraphValueToKey(value); err == nil {
				keys = append(keys[:len(keys):len(keys)], key)
			}
		}
	}
	return logrus.Fields{"write-op-type-id": w.OpTypeID, "object-keys": keys}
}

// result returns how the call's guarded writes end its op: a system error
// when the guard failed to decide a write, since the run was incomplete, else
// the first refusal, else nil.
func (g *handlerWriteGuard) result() (sysErr bool, err error) {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	if g.failed != nil {
		return true, g.failed
	}
	return false, g.refused
}

// getRecorded returns how many refused writes record mode let through.
func (g *handlerWriteGuard) getRecorded() int {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	return g.recorded
}

// _ is a type assertion
var _ world.WriteGuard = (*handlerWriteGuard)(nil)
