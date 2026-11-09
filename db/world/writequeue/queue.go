package writequeue

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/db/world/viewstream"
	"github.com/sirupsen/logrus"
)

// MaxBatch bounds the writes one World commit carries, so a batch's
// writer lock hold stays near a few commits' worth.
const MaxBatch = 64

// batchBodyBudget is how long a batch's bodies may run before it commits the
// writes run so far and leaves the rest for the next batch. Bodies run under
// the engine's writer lock, so a batch of slow writes, such as course results,
// would otherwise hold it, and every write queued behind it, for their sum.
const batchBodyBudget = 250 * time.Millisecond

// slowBatchWarn is how long a batch commits before the queue warns, and how
// often it warns again while the batch still commits. Every batch behind a
// stuck one waits for its result, so the warning names the stall before the
// writers time out.
const slowBatchWarn = 30 * time.Second

// slowBodyWarn is how long one write's checks and body run before the queue
// names the write in a warning.
const slowBodyWarn = time.Second

// batchAttemptLimit bounds the times the engine may refuse a write for a stale
// base or a failed predecessor before the write fails, as kvtx bounds a single
// write's attempts.
const batchAttemptLimit = 10

// ErrNestedWrite refuses a World write started inside another write's body,
// which would wait for the batch that runs it.
var ErrNestedWrite = errors.New("World write started inside another World write")

// Queue commits the World writes of every caller bound to one World in
// batches. The engine holds its writer lock from a write's fork until it
// admits the commit, so the writes that arrive while one batch runs share the
// next batch's fork, commit and install. Bodies run one at a time, in arrival
// order, each seeing the writes before it.
//
// The queue is a pipeline of one stage. The drain goroutine forks a batch,
// runs its bodies and has the engine admit its commit, then forks the next
// batch from the admitted World. A write that can run again without its
// writer, one applying an operation, succeeds at admission; any other waits
// for the batch's durable receipt, which a goroutine per batch waits for.
//
// An admitted write is a promise. When a publication fails, the engine drops
// it and every revision admitted after it and reinstalls the World before
// them; the queue's reinstall hook then puts the dropped batches' writes back
// at the head of the queue, under the engine's writer lock and so before the
// next fork. The queue keeps its writes in arrival order, which is the order
// they were admitted, so a write that runs again still precedes every write
// that arrived after it.
type Queue struct {
	// ctx owns admitted obligations independently of any request.
	ctx context.Context
	// engine opens the batch transactions.
	engine world.Engine
	// options binds transaction coordination and application observations.
	options Options
	// bodyBudget is how long one batch's bodies may run; see batchBodyBudget.
	bodyBudget time.Duration
	// owners records the revision each actor's writes committed, or is nil.
	owners *viewstream.Writes

	// mtx guards the fields below.
	mtx sync.Mutex
	// pending are the writes waiting for a batch, in arrival order.
	pending []*queuedWrite
	// arrivals counts the writes queued, numbering each.
	arrivals uint64
	// committing holds the batches whose commit is not durable yet, in
	// admission order.
	committing []*committingBatch
	// draining is set while a goroutine forks batches for the pending writes.
	draining bool
	// parked are the writes set aside while they fetch blocks from peers;
	// see park.
	parked []*queuedWrite
	// fetching counts each actor's parked writes; the actor's pending writes
	// wait while it is not zero.
	fetching map[uint64]int
}

// committingBatch is a batch from its commit until its durable result.
type committingBatch struct {
	// writes are the batch's writes.
	writes []*queuedWrite
	// oldest is when its oldest write queued.
	oldest time.Time
	// seqno is the World revision the engine admitted, or zero before.
	seqno uint64
	// waiting reports whether a writer waits for the durable result.
	waiting bool
	// dropped is set once the engine dropped the batch and its writes queued
	// again.
	dropped bool
}

// New constructs a queue that preserves admitted replay obligations across
// engine reinstalls. A successful durable answer never precedes WaitDurable;
// an admission answer requires a caller-supplied replay body. No stored format
// is changed. Preparation happens only after the failed attempt is discarded.
func New(engine world.Engine, options Options) *Queue {
	// Apply scheduling defaults and bind the transaction adapter.
	if options.Wrap == nil {
		options.Wrap = wrapBasic
	}
	budget := options.BodyBudget
	if budget <= 0 {
		budget = batchBodyBudget
	}
	q := &Queue{ctx: context.Background(), engine: engine, options: options, bodyBudget: budget, owners: options.Owners}

	// Requeue accepted obligations while the engine reinstalls its durable base.
	if _, err := world.OnReinstall(q.ctx, engine, q.reinstalled); err != nil {
		logrus.WithError(err).Warn("follow the World's reinstalls; dropped writes requeue when their receipts fail")
	}
	return q
}

// Write queues body, a write for actor or for no one actor when zero,
// behind the writes before it and returns the World revision it committed,
// or the context's error when ctx ends first; the write may still commit
// then, as a write cancelled during its commit may. replay, when not nil,
// lets the write succeed at admission; see queuedWrite.
func (q *Queue) Write(ctx context.Context, actor uint64, checks []Check, body, replay func(context.Context, world.Tx) error) (uint64, error) {
	// A write inside a running body would wait for its own batch.
	if ctx.Value(batchedWriteKey{}) == q {
		return 0, ErrNestedWrite
	}

	// Queue the write, starting a drain when none runs.
	w := &queuedWrite{ctx: ctx, queued: time.Now(), actor: actor, checks: checks, body: body, replay: replay, done: make(chan struct{})}
	w.name = func() string { return q.name(w.ctx) }

	// Number and enqueue the write before claiming its drain.
	q.mtx.Lock()
	q.arrivals++
	w.arrival = q.arrivals
	q.pending = append(q.pending, w)
	start := !q.draining
	q.draining = true
	q.mtx.Unlock()

	// Start the drain only for the caller that claimed it.
	if start {
		go q.drain()
	}

	// Wait for the batch that carries it.
	select {
	case <-w.done:
		return w.seqno, w.err
	case <-ctx.Done():
	}

	// A writer that leaves before its body starts keeps it from running; one
	// whose body started waits for it, so the body never touches the
	// writer's variables after write returns.
	w.runMtx.Lock()
	w.abandoned = true
	w.runMtx.Unlock()
	return 0, context.Cause(ctx)
}
