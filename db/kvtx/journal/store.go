package kvtx_journal

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/sirupsen/logrus"
)

// HeadKey is the key of the Head in the journaled store. No other writer uses
// it.
var HeadKey = []byte("\x00kvtx-journal/head")

// Default Options values.
const (
	defaultWait       = time.Second
	defaultStartWait  = 2 * time.Minute
	defaultFlushEvery = time.Second
	// sinkRetry is how often Open asks for an unreachable sink.
	sinkRetry = time.Second
)

// Write is one key's change in a commit.
type Write struct {
	// Key is the store key.
	Key []byte
	// Value is the new value, unused for a delete.
	Value []byte
	// Delete removes the key.
	Delete bool
}

// Record is one commit's writes, in the order the commit made them.
type Record struct {
	// Epoch names the run that committed it.
	Epoch uint64
	// Seq is the commit's place in the store's commit order.
	Seq uint64
	// Writes are the commit's changes.
	Writes []Write
}

// Sink is the remote journal.
type Sink interface {
	// Connected reports whether the sink can take records now.
	Connected() bool
	// Append returns nil once rec is durable in the sink. It returns
	// promptly when ctx ends.
	Append(ctx context.Context, rec *Record) error
	// Tail returns the sink's consecutive records of epoch past after.
	Tail(ctx context.Context, epoch, after uint64) ([]*Record, error)
	// Cut has the sink set aside every record except epoch's up to through,
	// and refuse later records of epoch.
	Cut(ctx context.Context, epoch, through uint64) error
	// Trim tells the sink the store holds epoch's records up to through
	// durably. It must not block.
	Trim(epoch, through uint64)
}

// Inner is the store a Store journals. Its write transactions should
// implement kvtx.OrderedCommitTx; one that does not commits with its own
// flush, which is still correct.
type Inner interface {
	kvtx.OrderedCommitStore
}

// Options configure a Store.
type Options struct {
	// Live is the switch: while it reports true and the sink is connected,
	// the journal goes live. Nil never does.
	Live func() bool
	// Wait bounds a commit's wait for the sink before it flushes locally.
	Wait time.Duration
	// StartWait bounds Open's wait for an unreachable sink after a run that
	// relied on it. Past it the store starts on its own durable head and has
	// the sink set aside that run's records once it is back.
	StartWait time.Duration
	// FlushEvery is how often the inner store is flushed in the background.
	FlushEvery time.Duration
	// Logger logs the journal's state changes, or is nil.
	Logger *logrus.Entry
}

// Store is a kvtx store whose commits are durable in a Sink while the
// journal is live. See the package comment.
type Store struct {
	inner Inner
	sink  Sink
	opts  Options
	le    *logrus.Entry
	// epoch names this run.
	epoch uint64

	// bcast guards the fields below and broadcasts when durability advances.
	bcast broadcast.Broadcast
	// seq is the seq of the last commit.
	seq uint64
	// local is the highest seq the inner store holds durably.
	local uint64
	// flushed holds the seqs past local the sink holds.
	flushed map[uint64]struct{}
	// reach is the highest seq whose commits past local the sink all holds,
	// at least local.
	reach uint64
	// live is the marker as durably set: commits record and may rely on the
	// sink.
	live bool
	// journaled records that this run went live, so its flushes trim the
	// sink; a run that never did sends the sink nothing.
	journaled bool
	// cut is the run whose records the sink must still set aside, zero for
	// none, and cutThrough the last of its seqs the store holds.
	cut, cutThrough uint64
}

// Open opens a Store over inner, replaying the sink's records past inner's
// durable head first when the head's marker is live. Run the background
// flush and marker with Run.
func Open(ctx context.Context, inner Inner, sink Sink, opts Options) (*Store, error) {
	// Fill in the defaults.
	if opts.Wait <= 0 {
		opts.Wait = defaultWait
	}
	if opts.StartWait <= 0 {
		opts.StartWait = defaultStartWait
	}
	if opts.FlushEvery <= 0 {
		opts.FlushEvery = defaultFlushEvery
	}
	le := opts.Logger
	if le == nil {
		le = logrus.NewEntry(logrus.StandardLogger())
	}
	epoch, err := newEpoch()
	if err != nil {
		return nil, err
	}
	s := &Store{inner: inner, sink: sink, opts: opts, le: le, epoch: epoch, flushed: make(map[uint64]struct{})}

	// Read the durable head. Every commit after a live one was journaled.
	head, err := s.readHead(ctx)
	if err != nil {
		return nil, err
	}
	s.seq = head.GetSeq()
	if !head.GetLive() {
		s.local, s.reach = s.seq, s.seq
		return s, nil
	}

	// Replay the run's records past the head, or start without them.
	records, err := s.tail(ctx, head.GetEpoch(), head.GetSeq())
	if err != nil {
		le.WithError(err).WithField("epoch", head.GetEpoch()).WithField("seq", head.GetSeq()).
			Error("journal unreachable at start: starting on the local head; commits acknowledged through the journal past it are set aside on the node, not applied")
	}
	for _, rec := range records {
		if err := s.replay(ctx, rec); err != nil {
			return nil, err
		}
	}
	if len(records) != 0 {
		le.WithField("records", len(records)).WithField("through", s.seq).Info("replayed the journal past the local head")
	}

	// Clear the marker for this run, durably, and have the sink set the old
	// run aside.
	if err := s.writeHead(ctx, false); err != nil {
		return nil, err
	}
	s.local, s.reach = s.seq, s.seq
	s.cut, s.cutThrough = head.GetEpoch(), s.seq
	s.cutOld(ctx)
	return s, nil
}

// newEpoch returns a random run name, never zero.
func newEpoch() (uint64, error) {
	var b [8]byte
	for {
		if _, err := rand.Read(b[:]); err != nil {
			return 0, err
		}
		if e := binary.BigEndian.Uint64(b[:]); e != 0 {
			return e, nil
		}
	}
}

// readHead returns the inner store's Head, empty when it has none.
func (s *Store) readHead(ctx context.Context) (*Head, error) {
	// Read it in a read transaction.
	tx, err := s.inner.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, HeadKey)
	if err != nil || !found {
		return &Head{}, err
	}
	head := &Head{}
	return head, head.UnmarshalVT(data)
}

// tail asks the sink for epoch's records past after, waiting up to StartWait
// for it to connect.
func (s *Store) tail(ctx context.Context, epoch, after uint64) ([]*Record, error) {
	deadline := time.Now().Add(s.opts.StartWait)
	for {
		if s.sink.Connected() {
			records, err := s.sink.Tail(ctx, epoch, after)
			if err == nil {
				return records, nil
			}
			if time.Now().After(deadline) {
				return nil, err
			}
		} else if time.Now().After(deadline) {
			return nil, errors.New("journal sink not connected")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(sinkRetry):
		}
	}
}

// replay applies rec in its own commit, which also moves the head to it.
func (s *Store) replay(ctx context.Context, rec *Record) error {
	// The sink returns consecutive records past the head.
	if rec.Seq != s.seq+1 {
		return errors.Errorf("journal record %d does not follow the head at %d", rec.Seq, s.seq)
	}
	tx, err := s.inner.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	for _, w := range rec.Writes {
		if w.Delete {
			err = tx.Delete(ctx, w.Key)
		} else {
			err = tx.Set(ctx, w.Key, w.Value)
		}
		if err != nil {
			return err
		}
	}
	if err := s.commitHead(ctx, tx, &Head{Epoch: rec.Epoch, Seq: rec.Seq, Live: true}); err != nil {
		return err
	}
	s.seq = rec.Seq
	return nil
}

// writeHead commits a Head of this run with the marker set to live in a
// commit of its own, and flushes it. The caller holds no write transaction.
//
// The marker's commit takes the writer turn, so every commit opened before it
// has committed. The in-memory marker changes before the commit, under the
// turn, so every commit after it in the commit order records exactly when the
// marker says so. Going live, a recorded commit after the marker cannot rely
// on the sink before the marker is flushed: the marker sends no record, so
// the sink's reach stops below it until the flush. Going local, every commit
// after the marker is unrecorded and so is flushed by its own commit or lost
// with the marker, never interleaved with a replayed record.
func (s *Store) writeHead(ctx context.Context, live bool) error {
	// Take the writer turn.
	tx, err := s.inner.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Change the marker in memory, then commit it, restoring it on failure.
	var seq uint64
	var was bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		seq, was = s.seq+1, s.live
		s.live = live
		s.journaled = s.journaled || live
	})
	err = s.commitHead(ctx, tx, &Head{Epoch: s.epoch, Seq: seq, Live: live})
	if err != nil {
		s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { s.live = was })
		return err
	}
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { s.seq = seq })

	// Flush it with everything before it.
	return s.Sync(ctx)
}

// commitHead sets head in tx and commits tx without a flush.
func (s *Store) commitHead(ctx context.Context, tx kvtx.Tx, head *Head) error {
	data, err := head.MarshalVT()
	if err != nil {
		return err
	}
	if err := tx.Set(ctx, HeadKey, data); err != nil {
		return err
	}
	return kvtx.CommitOrdered(ctx, tx)
}

// cutOld has the sink set aside the old run's records once it can, so they
// are never applied. It runs at Open and in Run until it succeeds.
func (s *Store) cutOld(ctx context.Context) {
	// Find the run to cut, if any, and wait for the sink.
	var epoch, through uint64
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { epoch, through = s.cut, s.cutThrough })
	if epoch == 0 || !s.sink.Connected() {
		return
	}

	// Cut it, and forget it once cut.
	cctx, cancel := context.WithTimeout(ctx, s.opts.Wait*10)
	defer cancel()
	if err := s.sink.Cut(cctx, epoch, through); err != nil {
		s.le.WithError(err).Warn("unable to cut the journal's previous run, trying again")
		return
	}
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if s.cut == epoch {
			s.cut = 0
		}
	})
}

// NewTransaction opens a transaction of the inner store. A write
// transaction opened while the marker is live records its writes.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	// Open the inner transaction; a read is the inner one.
	inner, err := s.inner.NewTransaction(ctx, write)
	if err != nil || !write {
		return inner, err
	}

	// The writer turn is held from here: the marker cannot change until this
	// transaction ends.
	var live bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { live = s.live })
	if !live {
		return inner, nil
	}
	return &tx{Tx: inner, s: s}, nil
}

// SupportsAtomicCommit reports whether the inner store commits atomically.
func (s *Store) SupportsAtomicCommit() bool {
	atomic, ok := s.inner.(kvtx.AtomicCommitStore)
	return ok && atomic.SupportsAtomicCommit()
}

// WritersWaiting reports whether a writer waits for the inner store's turn.
func (s *Store) WritersWaiting() bool {
	q, ok := s.inner.(kvtx.WriterQueueStore)
	return ok && q.WritersWaiting()
}

// Sync flushes the inner store, making every commit so far durable locally,
// and tells the sink it may drop their records.
func (s *Store) Sync(ctx context.Context) error {
	// Read the last commit first: the flush covers every commit before it.
	var seq uint64
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { seq = s.seq })
	if err := s.inner.Sync(ctx); err != nil {
		return err
	}
	s.setLocal(seq)
	return nil
}

// WaitDurable waits until every commit so far is durable, locally or in the
// sink.
func (s *Store) WaitDurable(ctx context.Context) error {
	var seq uint64
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { seq = s.seq })
	return s.waitDurable(ctx, seq)
}

// setLocal records that the inner store holds every commit up to seq, and
// lets the sink drop their records, also those of a run that went local.
func (s *Store) setLocal(seq uint64) {
	// Move local and reach past the flushed seqs.
	var trim bool
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// An older flush moves nothing.
		if seq <= s.local {
			return
		}

		// Forget the flushed seqs the flush covers, and advance.
		for f := range s.flushed {
			if f <= seq {
				delete(s.flushed, f)
			}
		}
		s.local = seq
		s.reach = max(s.reach, seq)
		s.advanceLocked()
		trim = s.journaled
		broadcast()
	})

	// Tell the sink.
	if trim {
		s.sink.Trim(s.epoch, seq)
	}
}

// setFlushed records that the sink holds seq.
func (s *Store) setFlushed(seq uint64) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if seq <= s.local {
			return
		}
		s.flushed[seq] = struct{}{}
		s.advanceLocked()
		broadcast()
	})
}

// advanceLocked moves reach over the flushed seqs that follow it. The caller
// holds bcast.
func (s *Store) advanceLocked() {
	for {
		if _, ok := s.flushed[s.reach+1]; !ok {
			return
		}
		delete(s.flushed, s.reach+1)
		s.reach++
	}
}

// waitDurable returns once every commit up to seq is durable: in the sink
// within Wait, or else locally by a flush.
func (s *Store) waitDurable(ctx context.Context, seq uint64) error {
	timer := time.NewTimer(s.opts.Wait)
	defer timer.Stop()
	for {
		var done bool
		var wait <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			done = s.local >= seq || s.reach >= seq
			wait = getWaitCh()
		})
		if done {
			return nil
		}
		if !s.sink.Connected() {
			return s.Sync(ctx)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return s.Sync(ctx)
		case <-wait:
		}
	}
}

// commit numbers a recorded transaction, commits it to the inner store
// without a flush, and sends its record. It returns the commit's seq.
func (s *Store) commit(ctx context.Context, t *tx) (uint64, error) {
	// Number the commit under the writer turn, which t holds.
	var seq uint64
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { seq = s.seq + 1 })
	if err := s.commitHead(ctx, t.Tx, &Head{Epoch: s.epoch, Seq: seq, Live: true}); err != nil {
		return 0, err
	}
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { s.seq = seq })

	// Send the record beside whatever waits for it, unless it is too large.
	if t.size > MaxRecordBytes {
		return seq, nil
	}
	rec := &Record{Epoch: s.epoch, Seq: seq, Writes: t.writes}
	go func() {
		actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.opts.Wait)
		defer cancel()
		if err := s.sink.Append(actx, rec); err == nil {
			s.setFlushed(seq)
		}
	}()
	return seq, nil
}

// Run flushes the inner store every FlushEvery, sets and clears the marker
// as the switch and the sink allow, and cuts the old run, until ctx ends.
func (s *Store) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.opts.FlushEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		s.step(ctx)
	}
}

// step runs one round of Run.
func (s *Store) step(ctx context.Context) {
	// Flush what the inner store has not.
	var seq, local, cut uint64
	var live bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		seq, local, live, cut = s.seq, s.local, s.live, s.cut
	})
	if seq > local {
		if err := s.Sync(ctx); err != nil {
			s.le.WithError(err).Warn("journal: local flush failed")
		}
	}
	if cut != 0 {
		s.cutOld(ctx)
	}

	// Move the marker. Going live needs the sink: a commit that relies on
	// it must reach it.
	want := s.opts.Live != nil && s.opts.Live()
	switch {
	case want && !live && cut == 0 && s.sink.Connected():
		if err := s.writeHead(ctx, true); err != nil {
			s.le.WithError(err).Warn("journal: unable to set the live marker")
			return
		}
		s.le.Info("journal live: commits are durable on the node")
	case !want && live:
		if err := s.writeHead(ctx, false); err != nil {
			s.le.WithError(err).Warn("journal: unable to clear the live marker")
			return
		}
		s.le.Info("journal off: commits are durable locally")
	}
}

// Close flushes the inner store, so a clean stop needs no replay.
func (s *Store) Close(ctx context.Context) error {
	return s.Sync(ctx)
}

// _ are type assertions
var (
	_ kvtx.OrderedCommitStore = (*Store)(nil)
	_ kvtx.AtomicCommitStore  = (*Store)(nil)
	_ kvtx.WriterQueueStore   = (*Store)(nil)
)
