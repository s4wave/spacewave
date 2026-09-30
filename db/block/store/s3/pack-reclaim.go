//go:build !tinygo

package block_store_s3

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	// reclaimDir holds the reclaim state under the object prefix.
	reclaimDir = "reclaim/"
	// reclaimPayback is the time within which the storage a reclaimed
	// packfile frees must repay the requests and egress of reclaiming it.
	reclaimPayback = 365 * 24 * time.Hour
	// reclaimMaxInterval is the longest time between reclaim passes.
	reclaimMaxInterval = 30 * 24 * time.Hour
)

// Reclaim drops the blocks live reports dead from the packfiles listed before
// fence runs, when that costs less than storing them.
//
// A pass runs only when due, as reclaimDue describes. It lists the entries,
// judges each listed packfile, and picks the ones worthReclaim accepts. When
// it picks none, it returns without calling fence. Otherwise it calls fence,
// checks the picked packfiles again, and rewrites each one still worth it
// with only its live blocks, or deletes it when none is live. A writer that
// may still reference a dead block must upload it again after fence returns,
// so that upload lands in a packfile the pass does not judge, and only the
// check after fence decides which blocks drop. live reports for each ref
// whether its block is still reachable.
//
// Compaction merges the rewritten packfiles later. A packfile another writer
// merged first is skipped; its blocks wait for the next pass.
func (s *PackStore) Reclaim(
	ctx context.Context,
	fence func(context.Context) error,
	live func(context.Context, []*block.BlockRef) ([]bool, error),
) error {
	// Exclude compaction for the pass.
	release, err := s.compactMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Skip the pass until it is due.
	now := time.Now()
	prev, err := s.loadReclaimState(ctx)
	if err != nil {
		return err
	}
	if !s.reclaimDue(prev, now) {
		return nil
	}

	// Judge the listed packfiles and pick the ones worth reclaiming.
	judged, err := s.judgeListed(ctx, live)
	if err != nil {
		return err
	}
	picked := slices.DeleteFunc(slices.Clone(judged), func(j *packJudgment) bool {
		return !s.worthReclaim(j, now)
	})

	// Fence the writers and reclaim the picked packfiles.
	dropped, reclaimErr := s.reclaimPicked(ctx, picked, fence, live, now)

	// Record the pass, even a failed one, so the next request waits until a
	// pass is due instead of scanning again. A failed pass keeps the dead
	// bytes of the previous one, which the next pass measures from.
	state := newReclaimState(prev, judged, dropped, now)
	if reclaimErr != nil {
		state.DeadBytes = prev.GetDeadBytes()
	}
	s.le.WithFields(logrus.Fields{
		"packs":              state.GetPacks(),
		"picked":             len(picked),
		"dropped-bytes":      dropped,
		"dead-bytes":         state.GetDeadBytes(),
		"dead-bytes-per-day": state.GetDeadBytesPerDay(),
	}).Info("storage reclaim pass")
	if err := s.saveReclaimState(ctx, state); err != nil && reclaimErr == nil {
		return err
	}
	return reclaimErr
}

// reclaimDue reports whether a pass is due. The first pass is due at once.
// Later, a pass is due when the blocks expected to die since prev would have
// cost as much to store as a pass costs to run, which minimizes the sum of the
// two, or when reclaimMaxInterval has passed.
func (s *PackStore) reclaimDue(prev *ReclaimState, now time.Time) bool {
	// Run the first pass, and any pass reclaimMaxInterval after the last.
	if prev == nil {
		return true
	}
	elapsed := max(now.Sub(prev.GetPassedAt().AsTime()), 0)
	if elapsed >= reclaimMaxInterval {
		return true
	}

	// Blocks die at a steady rate, so the ones that died were stored for half
	// the elapsed time on average.
	died := float64(prev.GetDeadBytesPerDay()) * elapsed.Hours() / 24
	kept := s.pricing.storageCost(died/2, elapsed)
	return kept >= s.pricing.scanCost(prev.GetPacks(), prev.GetBlocks())
}

// worthReclaim reports whether reclaiming the packfile of j pays for itself.
//
// Its dead blocks must hold at least half its value bytes, which bounds the
// bytes rewritten to the size of the blocks dropped. It must be older than
// the minimum storage duration of the service, since deleting it sooner saves
// nothing. The dead blocks must cost more to store for reclaimPayback than
// the requests and egress of reclaiming the packfile.
func (s *PackStore) worthReclaim(j *packJudgment, now time.Time) bool {
	// Keep a packfile mostly live or younger than the minimum storage.
	if j.dead == 0 || j.dead*2 < j.total {
		return false
	}
	if now.Sub(j.entry.GetCreatedAt().AsTime()) < s.pricing.MinStorage {
		return false
	}

	// Weigh the storage freed against the cost of freeing it.
	saved := s.pricing.storageCost(float64(j.dead), reclaimPayback)
	return saved > s.pricing.reclaimCost(j.entry.GetSizeBytes(), j.dead < j.total)
}

// packJudgment is the liveness of the blocks of one packfile.
type packJudgment struct {
	// entry is the packfile entry.
	entry *packfile.PackfileEntry
	// index is the packfile key index.
	index []*kvfile.IndexEntry
	// refs are the block refs of index, in order.
	refs []*block.BlockRef
	// alive reports for each ref whether its block is live.
	alive []bool
	// total is the value bytes of every block.
	total uint64
	// dead is the value bytes of the dead blocks.
	dead uint64
}

// judgeListed lists the entries and judges each listed packfile. A packfile
// another writer removed since the listing is skipped.
func (s *PackStore) judgeListed(
	ctx context.Context,
	live func(context.Context, []*block.BlockRef) ([]bool, error),
) ([]*packJudgment, error) {
	// Snapshot the listed packfiles.
	if err := s.listEntries(ctx, s.listings.Load()); err != nil {
		return nil, err
	}
	var listed []*packfile.PackfileEntry
	s.bcast.HoldLock(func(func(), func() <-chan struct{}) {
		listed = slices.Collect(maps.Values(s.entries))
	})

	// Judge them concurrently, since each reads its index from the bucket.
	judged := make([]*packJudgment, len(listed))
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(batchConcurrency)
	for i, entry := range listed {
		eg.Go(func() error {
			j, err := s.judgePack(egCtx, entry, live)
			if errors.Is(err, ErrNotFound) {
				return nil
			}
			judged[i] = j
			return err
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return slices.DeleteFunc(judged, func(j *packJudgment) bool { return j == nil }), nil
}

// judgePack reads the key index of the packfile of entry and asks live about
// each block. Returns ErrNotFound when the packfile is gone.
func (s *PackStore) judgePack(
	ctx context.Context,
	entry *packfile.PackfileEntry,
	live func(context.Context, []*block.BlockRef) ([]bool, error),
) (*packJudgment, error) {
	// Read the index and the ref of each block.
	index, err := s.readPackIndex(ctx, entry)
	if err != nil {
		return nil, err
	}
	refs := make([]*block.BlockRef, len(index))
	for i, ie := range index {
		h, err := packfile.ParseBlockKey(ie.GetKey())
		if err != nil {
			return nil, errors.Wrap(err, "packfile "+entry.GetId())
		}
		refs[i] = block.NewBlockRef(h)
	}

	// Check the blocks.
	j := &packJudgment{entry: entry, index: index, refs: refs}
	if err := j.check(ctx, live); err != nil {
		return nil, err
	}
	return j, nil
}

// check asks live about each block and sums the value bytes.
func (j *packJudgment) check(ctx context.Context, live func(context.Context, []*block.BlockRef) ([]bool, error)) error {
	// Ask live about each block.
	alive, err := live(ctx, j.refs)
	if err != nil {
		return errors.Wrap(err, "check live blocks")
	}
	if len(alive) != len(j.refs) {
		return errors.Errorf("live returned %d results for %d blocks", len(alive), len(j.refs))
	}

	// Sum the value bytes of every block and of the dead ones.
	j.alive, j.total, j.dead = alive, 0, 0
	for i, ie := range j.index {
		j.total += ie.GetSize()
		if !alive[i] {
			j.dead += ie.GetSize()
		}
	}
	return nil
}

// reclaimPicked runs fence, then checks each picked packfile again and
// reclaims it while that still pays. It does not run fence when nothing is
// picked. Returns the dead value bytes dropped.
func (s *PackStore) reclaimPicked(
	ctx context.Context,
	picked []*packJudgment,
	fence func(context.Context) error,
	live func(context.Context, []*block.BlockRef) ([]bool, error),
	now time.Time,
) (uint64, error) {
	// Fence the writers when there is work.
	if len(picked) == 0 {
		return 0, nil
	}
	if err := fence(ctx); err != nil {
		return 0, errors.Wrap(err, "reclaim fence")
	}

	// Reclaim each packfile still worth it.
	var dropped uint64
	for _, j := range picked {
		// Check it again now the writers are fenced.
		if err := j.check(ctx, live); err != nil {
			return dropped, err
		}
		if !s.worthReclaim(j, now) {
			continue
		}

		// Rewrite it, unless another writer merged it first.
		err := s.rewritePack(ctx, j)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return dropped, err
		}
		dropped += j.dead
	}
	return dropped, nil
}

// rewritePack writes the live blocks of j, in key order, as a new packfile and
// drops the old one, or drops it alone when no block is live.
func (s *PackStore) rewritePack(ctx context.Context, j *packJudgment) error {
	// Collect the keys of the live blocks.
	var keys []string
	for i, ie := range j.index {
		if j.alive[i] {
			keys = append(keys, string(ie.GetKey()))
		}
	}

	// Write them and drop the packfile.
	var output *packfile.PackfileEntry
	if len(keys) != 0 {
		values, err := s.readPack(ctx, j.entry.GetId())
		if err != nil {
			return err
		}
		output, err = s.writeValues(ctx, keys, values)
		if err != nil {
			return errors.Wrap(err, "write reclaimed packfile")
		}
	}
	return s.replace(ctx, output, []string{j.entry.GetId()})
}

// newReclaimState records a pass at now that judged judged and dropped
// dropped dead bytes.
//
// The death rate counts the dead bytes beyond those prev left, over the time
// since prev. The first pass measures from the oldest packfile.
func newReclaimState(prev *ReclaimState, judged []*packJudgment, dropped uint64, now time.Time) *ReclaimState {
	// Sum the judged packfiles and find the oldest.
	state := &ReclaimState{PassedAt: timestamppb.New(now), Packs: uint64(len(judged))}
	var dead uint64
	since := now
	for _, j := range judged {
		state.Blocks += uint64(len(j.index))
		dead += j.dead
		if created := j.entry.GetCreatedAt().AsTime(); created.Before(since) {
			since = created
		}
	}

	// Measure the rate blocks died since the previous pass, over at least an
	// hour.
	if prev != nil {
		since = prev.GetPassedAt().AsTime()
	}
	elapsed := max(now.Sub(since), time.Hour)
	died := dead - min(dead, prev.GetDeadBytes())
	state.DeadBytesPerDay = uint64(float64(died) * float64(24*time.Hour) / float64(elapsed))
	state.DeadBytes = dead - min(dead, dropped)
	return state
}

// loadReclaimState returns the last pass on the bucket prefix, reading it from
// the bucket once. Returns nil before the first pass. Called with compactMtx.
func (s *PackStore) loadReclaimState(ctx context.Context) (*ReclaimState, error) {
	// Return the state already read.
	if s.reclaimLoaded {
		return s.reclaimState, nil
	}

	// Find the latest state.
	keys, err := s.listReclaimStates(ctx)
	if err != nil || len(keys) == 0 {
		s.reclaimLoaded = err == nil
		return nil, err
	}

	// Read it.
	body, err := s.client.GetObject(ctx, s.bucket, keys[len(keys)-1])
	if err != nil {
		return nil, errors.Wrap(err, "read reclaim state")
	}
	data, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		return nil, errors.Wrap(err, "read reclaim state")
	}

	// Decode and keep it.
	state := &ReclaimState{}
	if err := state.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "decode reclaim state")
	}
	s.reclaimLoaded, s.reclaimState = true, state
	return state, nil
}

// saveReclaimState writes state as a new object named by its time and deletes
// the earlier ones with their versions, so a versioned bucket keeps no hidden
// copy. Called with compactMtx.
func (s *PackStore) saveReclaimState(ctx context.Context, state *ReclaimState) error {
	// Write the new state.
	data, err := state.MarshalVT()
	if err != nil {
		return err
	}
	key := s.prefix + reclaimDir + fmt.Sprintf("%016x", state.GetPassedAt().AsTime().UnixNano())
	if err := s.client.PutObject(ctx, s.bucket, key, data, "application/octet-stream"); err != nil {
		return errors.Wrap(err, "write reclaim state")
	}
	s.reclaimLoaded, s.reclaimState = true, state

	// Delete the earlier states.
	keys, err := s.listReclaimStates(ctx)
	if err != nil {
		return err
	}
	for _, old := range keys {
		if old == key {
			continue
		}
		if err := s.client.DeleteObject(ctx, s.bucket, old); err != nil {
			return errors.Wrap(err, "delete reclaim state")
		}
	}
	return nil
}

// listReclaimStates returns the keys of the reclaim state objects, oldest
// first.
func (s *PackStore) listReclaimStates(ctx context.Context) ([]string, error) {
	var keys []string
	err := s.client.ListObjects(ctx, s.bucket, s.prefix+reclaimDir, func(key string, _ int64) error {
		keys = append(keys, key)
		return nil
	})
	return keys, errors.Wrap(err, "list reclaim state")
}

// readPackIndex reads the key index of the packfile of entry through ranged
// reads, without its block values.
func (s *PackStore) readPackIndex(ctx context.Context, entry *packfile.PackfileEntry) ([]*kvfile.IndexEntry, error) {
	// Open the packfile for ranged reads.
	size := entry.GetSizeBytes()
	pack, err := s.openPack(entry.GetId(), int64(size)) //nolint:gosec // a packfile is at most writer.DefaultMaxPackBytes.
	if err != nil {
		return nil, err
	}
	defer pack.Close()

	// Read the index at its tail.
	_, tail, err := kvfile.ReadIndexTail(pack.ReaderAt(ctx), size)
	if err != nil {
		return nil, errors.Wrap(err, "read packfile index "+entry.GetId())
	}
	rd, err := kvfile.BuildReaderWithIndexTail(tail, size)
	if err != nil {
		return nil, errors.Wrap(err, "open packfile index "+entry.GetId())
	}

	// Collect its entries.
	var index []*kvfile.IndexEntry
	err = rd.ScanPrefixEntries(nil, func(ie *kvfile.IndexEntry, _ int) error {
		index = append(index, ie.CloneVT())
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "scan packfile index "+entry.GetId())
	}
	return index, nil
}
