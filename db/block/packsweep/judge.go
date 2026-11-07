package packsweep

import (
	"bytes"
	"context"
	"encoding/binary"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Defaults of JudgeOptions.
const (
	// DefaultMinAge is the youngest a pack may be to be judged.
	DefaultMinAge = 24 * time.Hour
	// DefaultLowLivePercent is the live share of a pack's value bytes at or
	// under which it is a compaction victim.
	DefaultLowLivePercent = 20
)

// judgeBatch is the packs judged in one read and written in one transaction.
const judgeBatch = 256

// Verdict is the judgment of a pack.
type Verdict byte

// The verdicts.
const (
	// VerdictKept is a pack whose live share is over the threshold.
	VerdictKept Verdict = iota
	// VerdictDead is a pack none of whose keys is live.
	VerdictDead
	// VerdictLow is a pack whose live share is at or under the threshold.
	VerdictLow
	// VerdictYoung is a pack younger than the minimum age, never judged.
	VerdictYoung
)

// String names the verdict.
func (v Verdict) String() string {
	switch v {
	case VerdictDead:
		return "dead"
	case VerdictLow:
		return "low"
	case VerdictYoung:
		return "young"
	default:
		return "kept"
	}
}

// Judgment is the judgment of one snapshot pack.
type Judgment struct {
	// ID is the pack.
	ID string
	// Verdict is the judgment.
	Verdict Verdict
	// Blocks is the keys the pack holds.
	Blocks uint64
	// LiveBlocks is its keys in L or W.
	LiveBlocks uint64
	// ValueBytes is the bytes of its values.
	ValueBytes uint64
	// LiveBytes is the bytes of its live values.
	LiveBytes uint64
	// Size is the pack's file size.
	Size uint64
	// Created is the pack's creation time.
	Created time.Time
}

// marshal encodes the judgment row.
func (j *Judgment) marshal() []byte {
	b := []byte{byte(j.Verdict)}
	for _, v := range []uint64{j.Blocks, j.LiveBlocks, j.ValueBytes, j.LiveBytes, j.Size} {
		b = binary.AppendUvarint(b, v)
	}
	return binary.AppendVarint(b, j.Created.Unix())
}

// unmarshalJudgment decodes a judgment row.
func unmarshalJudgment(id string, b []byte) (*Judgment, error) {
	r := &rowReader{b: b, ok: true}
	j := &Judgment{
		ID:         id,
		Verdict:    Verdict(r.byte()),
		Blocks:     r.uvarint(),
		LiveBlocks: r.uvarint(),
		ValueBytes: r.uvarint(),
		LiveBytes:  r.uvarint(),
		Size:       r.uvarint(),
		Created:    time.Unix(r.varint(), 0).UTC(),
	}
	return j, r.finish("judgment row")
}

// JudgeOptions configures Judge.
type JudgeOptions struct {
	// Store is the scratch store, holding a finished walk.
	Store kvtx.Store
	// MinAge is the youngest pack judged, against S. Zero uses DefaultMinAge.
	MinAge time.Duration
	// LowLivePercent is the live share at or under which a pack is a
	// compaction victim. Zero uses DefaultLowLivePercent; a negative value
	// finds no victim.
	LowLivePercent int
}

// Summary totals a judgment.
type Summary struct {
	// Packs counts the packs by verdict name.
	Packs map[string]uint64
	// Bytes sums the packs' file sizes by verdict name.
	Bytes map[string]uint64
	// DeadBytesInLow is the dead value bytes the low packs hold.
	DeadBytesInLow uint64
}

// Judge judges every snapshot pack against L and W, and writes each
// judgment. It refuses while W's frontier holds keys the closure has not
// walked. A pull running beside it can add to W during the judgment, which
// the recheck covers. It changes nothing but the cycle's judgment rows, so
// running it again rewrites the same ones.
func Judge(ctx context.Context, opts *JudgeOptions) (*Summary, error) {
	// Check the walk finished.
	head, err := ReadHead(ctx, opts.Store)
	if err != nil {
		return nil, err
	}
	if !head.Walked {
		return nil, errors.New("judge needs a finished walk")
	}
	if err := checkClosure(ctx, opts.Store); err != nil {
		return nil, err
	}

	// Take the bounds.
	minAge := opts.MinAge
	if minAge <= 0 {
		minAge = DefaultMinAge
	}
	lowPercent := opts.LowLivePercent
	if lowPercent == 0 {
		lowPercent = DefaultLowLivePercent
	}
	floor := head.SnapshotAt.Add(-minAge).Unix()

	// List the snapshot packs.
	packs, err := snapshotPacks(ctx, opts.Store)
	if err != nil {
		return nil, err
	}

	// Judge the packs a batch at a time, in one read, then write the batch's
	// rows in one transaction.
	sum := &Summary{Packs: make(map[string]uint64), Bytes: make(map[string]uint64)}
	for batch := range slices.Chunk(packs, judgeBatch) {
		judged, err := judgePacks(ctx, opts, batch, floor, lowPercent)
		if err != nil {
			return nil, err
		}
		if err := writeJudgments(ctx, opts.Store, judged); err != nil {
			return nil, err
		}
		for _, j := range judged {
			name := j.Verdict.String()
			sum.Packs[name]++
			sum.Bytes[name] += j.Size
			if j.Verdict == VerdictLow {
				sum.DeadBytesInLow += j.ValueBytes - j.LiveBytes
			}
		}
	}
	_, err = updateHead(ctx, opts.Store, func(h *Head) error {
		h.JudgedAt = time.Now().UTC()
		return nil
	})
	return sum, err
}

// checkClosure refuses a judgment while W's frontier holds keys.
func checkClosure(ctx context.Context, store kvtx.Store) error {
	// Read W's frontier.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	waiting, err := closureWaiting(ctx, tx)
	if err == nil && waiting {
		err = errors.New("judge needs the closure: pulled log keys wait in W")
	}
	return err
}

// snapshotPacks reads the cycle's snapshot rows.
func snapshotPacks(ctx context.Context, store kvtx.Store) ([]pendingPack, error) {
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	return scanSnapshot(ctx, tx)
}

// scanSnapshot reads the snapshot rows in tx.
func scanSnapshot(ctx context.Context, tx kvtx.TxOps) ([]pendingPack, error) {
	var out []pendingPack
	err := tx.ScanPrefix(ctx, []byte(snapshotPrefix), func(key, value []byte) error {
		row, err := unmarshalSnapshotPack(value)
		if err != nil {
			return err
		}
		out = append(out, pendingPack{id: string(key[len(snapshotPrefix):]), row: row})
		return nil
	})
	return out, err
}

// judgePacks judges a batch of packs in one read.
func judgePacks(ctx context.Context, opts *JudgeOptions, packs []pendingPack, floor int64, lowPercent int) ([]*Judgment, error) {
	// Open a read.
	tx, err := opts.Store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Judge each pack in it.
	out := make([]*Judgment, len(packs))
	for i, p := range packs {
		if out[i], err = judgePack(ctx, tx, p, floor, lowPercent); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// writeJudgments writes the judgments' rows in one transaction.
func writeJudgments(ctx context.Context, store kvtx.Store, judged []*Judgment) error {
	// Open a write.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Write each row, then commit them together.
	for _, j := range judged {
		if err := tx.Set(ctx, []byte(judgmentPrefix+j.ID), j.marshal()); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// judgePack counts a pack's live keys and returns its verdict.
func judgePack(ctx context.Context, tx kvtx.TxOps, p pendingPack, floor int64, lowPercent int) (*Judgment, error) {
	// Read the pack's catalog row.
	data, found, err := tx.Get(ctx, []byte(catalogPackPrefix+p.id))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.Errorf("pack %s is not in the catalog", p.id)
	}
	row, err := unmarshalCatalogPack(data)
	if err != nil {
		return nil, err
	}
	j := &Judgment{
		ID:         p.id,
		Blocks:     row.blocks,
		ValueBytes: row.valueBytes,
		Size:       row.size,
		Created:    time.Unix(row.created, 0).UTC(),
	}

	// Collect its members, then check each against L and W.
	type kv struct {
		tail []byte
		m    member
	}
	var members []kv
	prefix := []byte(catalogMemberPrefix + p.id + "/")
	err = tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		m, err := unmarshalMember(value)
		if err != nil {
			return err
		}
		members = append(members, kv{tail: bytes.Clone(key[len(prefix):]), m: m})
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, e := range members {
		live, err := isLive(ctx, tx, e.tail)
		if err != nil {
			return nil, err
		}
		if live {
			j.LiveBlocks++
			j.LiveBytes += e.m.valueSize
		}
	}

	// Decide: a pack too young or of unknown age is never judged.
	switch {
	case row.created == 0 || row.created > floor:
		j.Verdict = VerdictYoung
	case j.LiveBlocks == 0:
		j.Verdict = VerdictDead
	case lowPercent > 0 && j.LiveBytes*100 <= j.ValueBytes*uint64(lowPercent): //nolint:gosec // lowPercent is positive.
		j.Verdict = VerdictLow
	default:
		j.Verdict = VerdictKept
	}
	return j, nil
}

// isLive reports whether a key is in L or W.
func isLive(ctx context.Context, tx kvtx.TxOps, tail []byte) (bool, error) {
	live, err := tx.Exists(ctx, prefixed(visitedPrefix, tail))
	if err != nil || live {
		return live, err
	}
	return tx.Exists(ctx, prefixed(closureVisitedPrefix, tail))
}

// Judgments calls fn with each judgment of the cycle, in pack id order.
func Judgments(ctx context.Context, store kvtx.Store, fn func(*Judgment) error) error {
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	return tx.ScanPrefix(ctx, []byte(judgmentPrefix), func(key, value []byte) error {
		j, err := unmarshalJudgment(string(key[len(judgmentPrefix):]), value)
		if err != nil {
			return err
		}
		return fn(j)
	})
}
