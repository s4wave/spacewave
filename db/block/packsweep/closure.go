package packsweep

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/writelog"
	"github.com/s4wave/spacewave/db/kvtx"
)

// DefaultPullLines is the log lines one pull transaction stores when the
// options set none.
const DefaultPullLines = 16384

// DefaultPullEvery is how often FollowLog pulls when the options set none:
// well inside a backend's cap of minutes of writes.
const DefaultPullEvery = 10 * time.Second

// LogSource serves a backend's write log: its admin service, or, until that
// serves the log, the log directory.
type LogSource interface {
	// ReadLog calls fn with the lines after from, at most max when max is
	// positive.
	ReadLog(ctx context.Context, from writelog.Position, max int, fn func(writelog.Line) error) (writelog.ReadResult, error)
	// TrimLog deletes the lines at or before to.
	TrimLog(ctx context.Context, to writelog.Position) error
}

// LogPull is the state of the cycle's log pull.
type LogPull struct {
	// Start is the position R the roots were read at.
	Start string
	// Position is the last line stored.
	Position string
	// Lines is the lines stored.
	Lines uint64
	// Seeds is the keys the lines added to W.
	Seeds uint64
	// Gap is why the log stops covering the cycle from R, empty while it
	// covers it: an epoch that ended, or lines lost.
	Gap string
}

// marshal encodes the pull row.
func (p *LogPull) marshal() []byte {
	b := appendString(nil, p.Start)
	b = appendString(b, p.Position)
	b = appendUvarints(b, p.Lines, p.Seeds)
	return appendString(b, p.Gap)
}

// unmarshalLogPull decodes a pull row.
func unmarshalLogPull(b []byte) (*LogPull, error) {
	r := &rowReader{b: b, ok: true}
	p := &LogPull{Start: r.string(), Position: r.string(), Lines: r.uvarint(), Seeds: r.uvarint(), Gap: r.string()}
	return p, r.finish("log pull row")
}

// ReadLogPull reads the cycle's log pull, nil before the first pull.
func ReadLogPull(ctx context.Context, store kvtx.Store) (*LogPull, error) {
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	return readLogPull(ctx, tx)
}

// readLogPull reads the pull row in tx, nil when none exists.
func readLogPull(ctx context.Context, tx kvtx.TxOps) (*LogPull, error) {
	data, found, err := tx.Get(ctx, []byte(logPullKey))
	if err != nil || !found {
		return nil, err
	}
	return unmarshalLogPull(data)
}

// PullOptions configures PullLog.
type PullOptions struct {
	// Store is the scratch store.
	Store kvtx.Store
	// Source serves the log.
	Source LogSource
	// Start is the position R the roots were read at. The first pull of a
	// cycle records it; a later one must pass it again or leave it empty.
	Start string
	// Lines is the lines per transaction; zero uses DefaultPullLines.
	Lines int
}

// PullLog stores the log's lines since the last pull as W's seeds, a batch per
// transaction, and trims each stored batch from the source, until it has read
// every line written or the log stops covering the cycle. A pull killed at any
// point stores the same lines when run again, as the source keeps every line
// past the last trim.
func PullLog(ctx context.Context, opts *PullOptions) (*LogPull, error) {
	// Start the pull, or check it continues from the same R.
	pull, err := startPull(ctx, opts)
	if err != nil || pull.Gap != "" {
		return pull, err
	}
	lines := opts.Lines
	if lines <= 0 {
		lines = DefaultPullLines
	}

	// Store a batch at a time until the source has no more.
	for {
		res, err := pullBatch(ctx, opts, pull, lines)
		if err != nil {
			return nil, err
		}
		if err := opts.Source.TrimLog(ctx, res.Next); err != nil {
			return nil, errors.Wrap(err, "trim the log")
		}
		if res.State != writelog.ReadMore {
			return pull, nil
		}
	}
}

// startPull reads the pull row, writing it from opts.Start for the first
// pull.
func startPull(ctx context.Context, opts *PullOptions) (*LogPull, error) {
	// Read the row, and check a given start matches it.
	pull, err := ReadLogPull(ctx, opts.Store)
	if err != nil {
		return nil, err
	}
	if pull != nil {
		if opts.Start != "" && opts.Start != pull.Start {
			return nil, errors.Errorf("log pull started at %s, not %s", pull.Start, opts.Start)
		}
		return pull, nil
	}

	// Write the first row.
	if _, err := writelog.ParsePosition(opts.Start); err != nil {
		return nil, errors.Wrap(err, "the first log pull needs the position R")
	}
	pull = &LogPull{Start: opts.Start, Position: opts.Start}
	tx, err := opts.Store.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte(logPullKey), pull.marshal()); err != nil {
		return nil, err
	}
	return pull, tx.Commit(ctx)
}

// pullBatch reads up to lines lines after the pull's position and stores
// their keys in W, with the new position, in one transaction.
func pullBatch(ctx context.Context, opts *PullOptions, pull *LogPull, lines int) (writelog.ReadResult, error) {
	// Read the lines.
	from, err := writelog.ParsePosition(pull.Position)
	if err != nil {
		return writelog.ReadResult{}, err
	}
	var refs []*block.BlockRef
	res, err := opts.Source.ReadLog(ctx, from, lines, func(l writelog.Line) error {
		refs = append(refs, &block.BlockRef{Hash: l.Hash})
		return nil
	})
	if err != nil {
		return res, errors.Wrap(err, "read the log")
	}

	// Seed W with each key, and record the position and any gap.
	tx, err := opts.Store.NewTransaction(ctx, true)
	if err != nil {
		return res, err
	}
	defer tx.Discard()
	next := *pull
	for _, ref := range refs {
		added, err := visit(ctx, tx, closureSet, ref)
		if err != nil {
			return res, err
		}
		if added {
			next.Seeds++
		}
	}
	next.Lines += uint64(len(refs))
	next.Position = res.Next.String()
	next.Gap = pullGap(res)

	// Commit.
	if err := tx.Set(ctx, []byte(logPullKey), next.marshal()); err != nil {
		return res, err
	}
	if err := tx.Commit(ctx); err != nil {
		return res, err
	}
	*pull = next
	return res, nil
}

// pullGap returns why a read ends the log's cover of the cycle, empty when it
// does not: a cycle needs one epoch from R on, so a closed epoch ends it too.
func pullGap(res writelog.ReadResult) string {
	switch res.State {
	case writelog.ReadGap:
		return res.Reason
	case writelog.ReadClosed:
		return "epoch closed: the log was turned off or its process stopped"
	default:
		return ""
	}
}

// FollowLog pulls the log every interval, zero for DefaultPullEvery, until
// ctx ends or the log stops covering the cycle, keeping the backend's copy
// within its cap while the cycle's other steps run.
func FollowLog(ctx context.Context, opts *PullOptions, every time.Duration) (*LogPull, error) {
	if every <= 0 {
		every = DefaultPullEvery
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		pull, err := PullLog(ctx, opts)
		if err != nil || pull.Gap != "" {
			return pull, err
		}
		select {
		case <-ctx.Done():
			return pull, nil
		case <-t.C:
		}
	}
}

// ClosureOptions configures Closure.
type ClosureOptions struct {
	// Store is the scratch store, holding a finished walk.
	Store kvtx.Store
	// Refs serves blocks the catalog lacks; nil makes each a missing hole.
	Refs RefSource
	// Batch is the frontier keys per transaction. Zero uses
	// DefaultWalkBatch.
	Batch int
}

// Closure walks W from the keys the log pull stored, through the catalog and
// the ref source as the walk does, stopping at L: a key of L has its subtree
// in L. A put that re-links an old block brings the block and what it reaches
// into W. Run again after a later pull, it walks only the new keys.
func Closure(ctx context.Context, opts *ClosureOptions) (*Head, error) {
	// Check the walk finished from the R the pull started at.
	head, err := ReadHead(ctx, opts.Store)
	if err != nil {
		return nil, err
	}
	if !head.Walked {
		return nil, errors.New("closure needs a finished walk")
	}
	pull, err := ReadLogPull(ctx, opts.Store)
	if err != nil {
		return nil, err
	}
	if pull == nil {
		return head, nil
	}
	if pull.Start != head.LogPosition {
		return nil, errors.Errorf("log pull started at %s, the walk's roots at %s", pull.Start, head.LogPosition)
	}

	// Walk W's frontier until it is empty.
	batch := opts.Batch
	if batch <= 0 {
		batch = DefaultWalkBatch
	}
	w := &walker{store: opts.Store, refs: opts.Refs, set: closureSet}
	for {
		done, _, err := w.batch(ctx, batch)
		if err != nil {
			return nil, err
		}
		if done {
			return ReadHead(ctx, opts.Store)
		}
	}
}

// Reasons the log does not cover a cycle, each blocking its removal.
const (
	// BlockNoLog is a walk with no log position R.
	BlockNoLog = "no-log"
	// BlockLogUnread is a log position R with no pull since.
	BlockLogUnread = "log-unread"
	// BlockLogGap is a pull that found the log stopped covering the cycle.
	BlockLogGap = "log-gap"
)

// LogBlocking returns why the log does not cover the cycle from its R, empty
// when it does. A pack the roots do not reach may hold a block put since R, so
// only a cycle the log covers may remove anything.
func LogBlocking(head *Head, pull *LogPull) string {
	switch {
	case head.LogPosition == "":
		return BlockNoLog
	case pull == nil:
		return BlockLogUnread
	case pull.Gap != "":
		return BlockLogGap
	default:
		return ""
	}
}

// closureWaiting reports whether W's frontier holds a key in tx.
func closureWaiting(ctx context.Context, tx kvtx.TxOps) (bool, error) {
	errFound := errors.New("found")
	err := tx.ScanPrefixKeys(ctx, []byte(closureFrontierPrefix), func([]byte) error { return errFound })
	if err == errFound {
		return true, nil
	}
	return false, err
}
