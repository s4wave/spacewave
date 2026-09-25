package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"slices"

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
		if err := e.Apply(ctx, []*Record{{Key: key, Deleted: true}}); err != nil {
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
	entry, err := decodeJournalRecord(records[0])
	if err != nil {
		return nil, nil, err
	}
	return records[0].Key, entry, nil
}

// GetPendingOutgoingRefs scans the unreplayed journal in one snapshot for
// edges from node. Journal order applies each removal after earlier additions.
func (e *Engine) GetPendingOutgoingRefs(ctx context.Context, node string) ([]string, error) {
	read, err := e.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	defer read.release()

	var targets []string
	key, exclusive := []byte(journalPrefix), false
	for {
		records, err := read.seekEntries(ctx, key, exclusive, false)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			if !bytes.HasPrefix(record.Key, []byte(journalPrefix)) {
				return targets, nil
			}
			entry, err := decodeJournalRecord(record)
			if err != nil {
				return nil, err
			}
			for _, edge := range entry.Adds {
				if edge.Subject == node {
					targets = append(targets, edge.Object)
				}
			}
			for _, edge := range entry.Removes {
				if edge.Subject == node {
					targets = slices.DeleteFunc(targets, func(target string) bool { return target == edge.Object })
				}
			}
		}
		if len(records) == 0 {
			return targets, nil
		}
		key, exclusive = records[len(records)-1].Key, true
	}
}

// decodeJournalRecord decodes and bounds one journal record.
func decodeJournalRecord(record *Record) (*JournalEntry, error) {
	if len(record.Key) != len(journalPrefix)+8 {
		return nil, ErrCorrupt
	}
	entry := new(JournalEntry)
	if err := decode(record.Value, entry); err != nil {
		return nil, err
	}
	if len(entry.Adds)+len(entry.Removes) > maxBatchRecords {
		return nil, ErrCorrupt
	}
	for _, edges := range [][]*Edge{entry.Adds, entry.Removes} {
		for _, edge := range edges {
			if edge == nil || len(edge.Subject)+len(edge.Object)+10 > maxKeyBytes {
				return nil, ErrCorrupt
			}
		}
	}
	return entry, nil
}

// AcquireSTW excludes new journal appends during the collector's final sweep.
func (e *Engine) AcquireSTW(ctx context.Context) (func(), error) {
	return e.backend.Lock(ctx, "gc-stw", true)
}

// _ verifies the GC store's existing durable appender contract.
var _ block_gc.WALAppender = (*Engine)(nil)
