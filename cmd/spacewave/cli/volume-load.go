//go:build !js

package spacewave_cli

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	mrand "math/rand/v2"
	"slices"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
)

const (
	// volumeLoadBucket is the bucket of the load's scratch named root.
	volumeLoadBucket = "spacewave-debug-volume-load"
	// volumeLoadRoot is the load's scratch named root.
	volumeLoadRoot = "load"
	// volumeLoadLeaves is the number of leaf blocks each commit stores.
	volumeLoadLeaves = 8
	// volumeLoadLeafSize is the size of each leaf block in bytes.
	volumeLoadLeafSize = 3200
)

// volumeLoadOpts sets the writer an offline collection runs beside its
// passes.
type volumeLoadOpts struct {
	// rate is the writer's commits per second.
	rate float64
	// window is how long the writer runs, zero for no writer.
	window time.Duration
}

// volumeLoad is a writer that commits, at a set rate, a new tree of
// volumeLoadLeaves leaves and a parent referencing them, stored and held in one
// transaction that moves the scratch named root onto the parent, so the tree
// the root held before becomes garbage and the collector has steady work. The
// writer records each commit's latency, from the transaction's start to its
// return.
//
// The scratch root lives in its own bucket and holds no lease, so a writer
// killed before release leaves the root. The next load starts from the tree
// the root holds and releases the root at the end.
type volumeLoad struct {
	// vol is the Volume the load writes.
	vol *volume_s4db.Volume
	// interval is the time between commits.
	interval time.Duration
	// head is the parent the scratch root holds, nil when the root is free.
	head *block.BlockRef
	// fill generates the leaves' content, seeded once per load.
	fill *mrand.ChaCha8
	// latencies holds each commit's latency, in commit order.
	latencies []time.Duration
	// elapsed is how long the load ran.
	elapsed time.Duration
}

// newVolumeLoad returns a load on vol committing rate times per second,
// starting from the tree a killed load left under the scratch root.
func newVolumeLoad(ctx context.Context, vol *volume_s4db.Volume, rate float64) (*volumeLoad, error) {
	// Require a rate a ticker can keep.
	if rate <= 0 {
		return nil, errors.New("the load rate must be positive")
	}

	// Seed the leaves' content, so no run repeats another's blocks.
	var seed [32]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return nil, err
	}

	// Start from the tree the scratch root holds.
	head, err := vol.GetBucketRoot(ctx, volumeLoadBucket, volumeLoadRoot)
	if err != nil {
		return nil, errors.Wrap(err, "read the scratch root")
	}
	return &volumeLoad{
		vol:      vol,
		interval: time.Duration(float64(time.Second) / rate),
		head:     head,
		fill:     mrand.NewChaCha8(seed),
	}, nil
}

// run commits at the load's rate until ctx ends, kicking kick without
// blocking after each commit, and returns the first commit's error. A commit
// that overruns its interval delays the next. The ticker drops the ticks it
// missed, and the target count shows those missed ticks.
func (l *volumeLoad) run(ctx context.Context, kick chan<- struct{}) error {
	// Commit on each tick until the window ends. Each tick is the deadline
	// of the next commit the rate schedules. The ticker watches no state.
	start := time.Now()
	defer func() { l.elapsed = time.Since(start) }()
	ticker := time.NewTicker(l.interval) //gostyle:timer deadline
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		// Commit, then wake the collector.
		if err := l.commit(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case kick <- struct{}{}:
		default:
		}
	}
}

// commit stores a new tree and moves the scratch root onto its parent in one
// transaction, and records the transaction's latency.
func (l *volumeLoad) commit(ctx context.Context) error {
	// Build the leaves and the parent referencing them.
	entries := make([]*block.PutBatchEntry, 0, volumeLoadLeaves+1)
	refs := make([]*block.BlockRef, 0, volumeLoadLeaves)
	for range volumeLoadLeaves {
		data := make([]byte, volumeLoadLeafSize)
		_, _ = l.fill.Read(data)
		entry, err := volumeLoadEntry(data, nil)
		if err != nil {
			return err
		}
		entries, refs = append(entries, entry), append(refs, entry.Ref)
	}
	parent, err := volumeLoadEntry(binary.BigEndian.AppendUint64([]byte("load parent "), l.fill.Uint64()), refs)
	if err != nil {
		return err
	}
	entries = append(entries, parent)

	// Store the tree and move the root onto it, timing the transaction.
	move := block.NamedRoot{Name: volumeLoadRoot, Ref: parent.Ref, Previous: l.head}
	start := time.Now()
	if err := l.vol.SetBucketRoots(ctx, volumeLoadBucket, entries, []block.NamedRoot{move}); err != nil {
		return errors.Wrap(err, "commit a load tree")
	}
	l.latencies = append(l.latencies, time.Since(start))
	l.head = parent.Ref
	return nil
}

// release frees the scratch root, leaving its last tree to the collector.
func (l *volumeLoad) release(ctx context.Context) error {
	// A load that never committed and found no root holds nothing.
	if l.head == nil {
		return nil
	}
	move := block.NamedRoot{Name: volumeLoadRoot, Previous: l.head}
	if err := l.vol.SetBucketRoots(ctx, volumeLoadBucket, nil, []block.NamedRoot{move}); err != nil {
		return errors.Wrap(err, "release the scratch root")
	}
	l.head = nil
	return nil
}

// fields formats the load's commit count and latency distribution.
func (l *volumeLoad) fields() [][2]string {
	// Sort the latencies for the percentiles, and count the slow ones.
	sorted := slices.Sorted(slices.Values(l.latencies))
	var over5ms, over100ms int
	for _, d := range sorted {
		if d > 5*time.Millisecond {
			over5ms++
		}
		if d > 100*time.Millisecond {
			over100ms++
		}
	}

	// Format the table, with the count the rate called for.
	target := int(l.elapsed / l.interval)
	return [][2]string{
		{"Load commits", strconv.Itoa(len(sorted))},
		{"Load target commits", strconv.Itoa(target)},
		{"Load window", l.elapsed.Round(time.Millisecond).String()},
		{"Commit p50", volumeLoadPercentile(sorted, 50).String()},
		{"Commit p99", volumeLoadPercentile(sorted, 99).String()},
		{"Commit max", volumeLoadPercentile(sorted, 100).String()},
		{"Commits over 5ms", strconv.Itoa(over5ms)},
		{"Commits over 100ms", strconv.Itoa(over100ms)},
	}
}

// volumeLoadEntry returns a put entry for a block holding data and referencing
// refs.
func volumeLoadEntry(data []byte, refs []*block.BlockRef) (*block.PutBatchEntry, error) {
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		return nil, err
	}
	return &block.PutBatchEntry{Ref: ref, Data: data, Refs: refs, RefsKnown: true}, nil
}

// volumeLoadPercentile returns the p-th percentile of sorted by the
// nearest-rank method, or zero when it is empty.
func volumeLoadPercentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := (p*len(sorted) + 99) / 100
	return sorted[max(rank, 1)-1]
}
