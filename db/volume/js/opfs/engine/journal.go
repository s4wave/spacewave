package engine

import (
	"bytes"
	"context"
	"encoding/binary"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/volume/workload"
)

// journalPrefix orders unreplayed ownership transitions by durable sequence.
const journalPrefix = "\x03w"

// Append durably journals one ownership transition while sharing the GC barrier.
// Sequence allocation and the journal record publish in the same engine root.
func (e *Engine) Append(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	if len(adds) == 0 && len(removes) == 0 {
		return nil
	}
	if len(adds)+len(removes) > maxBatchRecords {
		return ErrLimit
	}
	entry := new(JournalEntry)
	for _, edges := range [][]block_gc.RefEdge{adds, removes} {
		for _, edge := range edges {
			if len(edge.Subject)+len(edge.Object)+10 > maxKeyBytes {
				return ErrLimit
			}
		}
	}
	for _, edge := range adds {
		entry.Adds = append(entry.Adds, &Edge{Subject: edge.Subject, Object: edge.Object})
	}
	for _, edge := range removes {
		entry.Removes = append(entry.Removes, &Edge{Subject: edge.Subject, Object: edge.Object})
	}
	data, err := encode(entry)
	if err != nil {
		return err
	}
	if len(data) > MaxValueBytes {
		return ErrLimit
	}
	workload.Record{Op: workload.OpJournalAppend, Size: int64(len(data))}.Log(ctx)
	release, err := e.backend.Lock(ctx, "gc-stw", false)
	if err != nil {
		return err
	}
	defer release()
	return e.mutate(ctx, func(_ *snapshot, p *publication) ([]*Record, error) {
		p.root.Revision++
		p.root.JournalSequence++
		key := binary.BigEndian.AppendUint64([]byte(journalPrefix), p.root.JournalSequence)
		return []*Record{{Key: key, Value: data}}, nil
	})
}

// ReplayWAL serializes ordered replay and removes each entry only after application.
// Repeating an entry after interruption is safe because graph updates are idempotent.
func (e *Engine) ReplayWAL(ctx context.Context, graph block_gc.CollectorGraph) (count int, err error) {
	defer func() { workload.Record{Op: workload.OpJournalReplay, Size: int64(count)}.Log(ctx) }()
	release, err := e.backend.Lock(ctx, "gc-replay", true)
	if err != nil {
		return 0, err
	}
	defer release()
	// Replay only entries present at this fence so concurrent appenders cannot starve GC.
	read, err := e.snapshot(ctx)
	if err != nil {
		return 0, err
	}
	fence := read.root.JournalSequence
	read.release()
	for {
		key, entry, err := e.nextJournalEntry(ctx)
		if err != nil || entry == nil {
			return count, err
		}
		if binary.BigEndian.Uint64(key[len(journalPrefix):]) > fence {
			return count, nil
		}
		adds := make([]block_gc.RefEdge, len(entry.Adds))
		removes := make([]block_gc.RefEdge, len(entry.Removes))
		for i, edge := range entry.Adds {
			adds[i] = block_gc.RefEdge{Subject: edge.Subject, Object: edge.Object}
		}
		for i, edge := range entry.Removes {
			removes[i] = block_gc.RefEdge{Subject: edge.Subject, Object: edge.Object}
		}
		if err := graph.ApplyRefBatch(ctx, adds, removes); err != nil {
			return count, err
		}
		if err := e.Apply(ctx, nil, []*Record{{Key: key, Deleted: true}}); err != nil {
			return count, err
		}
		count++
	}
}

// nextJournalEntry copies one bounded record before releasing file protection.
func (e *Engine) nextJournalEntry(ctx context.Context) ([]byte, *JournalEntry, error) {
	read, err := e.snapshot(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer read.release()
	records, err := read.seekEntries(ctx, []byte(journalPrefix), false, false)
	if err != nil {
		return nil, nil, err
	}
	if len(records) == 0 || !bytes.HasPrefix(records[0].Key, []byte(journalPrefix)) {
		return nil, nil, nil
	}
	if len(records[0].Key) != len(journalPrefix)+8 {
		return nil, nil, ErrCorrupt
	}
	entry := new(JournalEntry)
	if err := decode(records[0].Value, entry); err != nil {
		return nil, nil, err
	}
	if len(entry.Adds)+len(entry.Removes) > maxBatchRecords {
		return nil, nil, ErrCorrupt
	}
	for _, edges := range [][]*Edge{entry.Adds, entry.Removes} {
		for _, edge := range edges {
			if edge == nil || len(edge.Subject)+len(edge.Object)+10 > maxKeyBytes {
				return nil, nil, ErrCorrupt
			}
		}
	}
	return records[0].Key, entry, nil
}

// AcquireSTW excludes new journal appends during the collector's final sweep.
func (e *Engine) AcquireSTW(ctx context.Context) (func(), error) {
	return e.backend.Lock(ctx, "gc-stw", true)
}

// _ verifies the GC store's existing durable appender contract.
var _ block_gc.WALAppender = (*Engine)(nil)
