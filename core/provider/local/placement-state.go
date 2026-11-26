package provider_local

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_filesegment "github.com/s4wave/spacewave/db/block/store/filesegment"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	"github.com/sirupsen/logrus"
)

// placementState is a block store's storage backend, its upload queue, and
// the stores a storage move copies blocks between. One mount builds it with
// newPlacementState; after that only its backend, its read counts and its
// eviction state change.
type placementState struct {
	// wb queues local writes for upload.
	wb *block_store_writeback.Store
	// backend is the storage backend holding the store, nil for the account's
	// own storage. It changes before wb broadcasts the change.
	backend atomic.Pointer[account_settings.StorageBackend]
	// local is the account's own storage, written without upload markers.
	local block.StoreOps
	// remote is the open storage backend store, or nil.
	remote *atomic.Pointer[openBackend]
	// remoteBcast serializes publication of backend and remote with the
	// capture of their change channel. The tracker owns it across mounts.
	remoteBcast *broadcast.Broadcast
	// tracker reports the blocks the volume keeps child lists for, or nil when
	// the volume keeps none.
	tracker childListReader
	// done closes when this mount ends, including a failed mount or remount.
	done <-chan struct{}
	// serveOnly is serve mode, as SetServeOnly describes: reads fill nothing.
	serveOnly bool
	// peerUpload stores peer reads with an upload marker, as
	// SetPeerFillsUpload describes.
	peerUpload bool
	// peers is the store's peer reads, which count their outcomes in
	// peerReads. In serve mode it serves nothing.
	peers block_store.StoreSource
	// reads records the blocks read since the last eviction pass.
	reads readSet
	// localHits counts the blocks the local store served.
	localHits atomic.Uint64
	// peerReads counts the peer reads by outcome and the blocks they filled.
	peerReads peerReadCounts
	// evictGate is held by each eviction pass and by a storage move.
	evictGate *csync.Mutex
	// evictAdmit paces eviction passes, or holds nil.
	evictAdmit *atomic.Pointer[block_store_writeback.EvictAdmit]
	// resident is the account's resident mark override, or nil.
	resident *residentOverride
	// policyWake wakes the eviction loop when the policy changes; nil never.
	policyWake chan struct{}
	// evictLatch carries the eviction loop to the low mark across passes and
	// routine restarts, as EvictLatch describes; only runEviction uses it.
	evictLatch block_store_writeback.EvictLatch
	// sqlPrimary reports that the store writes to its SQL backend, which
	// then holds blocks the reference graph may not track.
	sqlPrimary bool
	// fallbackFills runs and counts the fills of the SQL read fallback.
	fallbackFills fallbackFills
	// cache is the fill cache that takes the blocks read from the backend, or nil
	// when they fill the local store. A store mounted without a backend has none.
	cache FillCache
	// front is read after the local store misses and before the fill cache
	// and the backend, or nil.
	front block.StoreOps
	// cold is the read-only cold pack tier behind the backend, or nil.
	cold block.StoreOps
	// drain uploads the finalized packs of the store's packahead log, or is
	// nil when the store has no log.
	drain *packahead.Drain
	// fileSegments serves the blocks of linked files from the files, after the
	// local store misses and before every remote tier.
	fileSegments *block_store_filesegment.Store
	// packed is the store over the packahead log, or nil when the store has
	// no log.
	packed *block_store_writeback.PackedStore
}

// placementMount is what one mount of a block store opened for its placement.
type placementMount struct {
	// wb queues the mount's writes for upload.
	wb *block_store_writeback.Store
	// local is the store the mount reads first: the packahead log over the
	// Volume's bucket, or the bucket.
	local block.StoreOps
	// packs is the store's packahead log, or nil.
	packs *storePacks
	// files serves the blocks of linked files.
	files *block_store_filesegment.Store
	// backend is the storage backend the account placed the store on when it
	// mounted, or nil.
	backend *account_settings.StorageBackend
	// cold is the cold pack tier behind the backend, or nil.
	cold block.StoreOps
	// serve is serve mode, read once for the mount.
	serve bool
}

// newPlacementState builds the placement of one mount of t's store, which
// ends when ctx does. It loads the saved block reads, starts the classifier of
// the blocks peers serve, which runs until ctx ends, and publishes m.backend
// to the waiters of t's backend.
func newPlacementState(ctx context.Context, le *logrus.Entry, t *bstoreTracker, m placementMount) *placementState {
	// Assemble the placement from the mount's stores and the tracker's
	// cross-mount state.
	p := &placementState{
		wb:            m.wb,
		local:         m.local,
		remote:        &t.remote,
		remoteBcast:   &t.remoteBcast,
		done:          ctx.Done(),
		serveOnly:     m.serve,
		peerUpload:    !m.serve && peerFillsUpload.Load(),
		evictGate:     &t.evictGate,
		evictAdmit:    &t.a.evictAdmit,
		resident:      &t.a.resident,
		policyWake:    make(chan struct{}, 1),
		sqlPrimary:    t.a.sqlPrimary.Load() && m.backend.GetSql() != nil,
		fallbackFills: fallbackFills{ctx: ctx, budget: &t.a.t.p.fillBudget},
		cold:          m.cold,
		fileSegments:  m.files,
	}
	p.tracker, _ = t.a.vol.(childListReader)
	if m.packs != nil {
		p.drain, p.packed = m.packs.drain, m.packs.packed
	}

	// A placed store reads the pack front first and, unless it serves only,
	// fills the account's fill cache.
	if m.backend != nil {
		if !m.serve {
			p.cache = t.fillCache()
		}
		p.front = t.packFront()
	}

	// Read peers, counting their outcomes, unless in serve mode.
	p.peers = func() block.StoreOps { return nil }
	if !m.serve {
		p.peers = t.peerSource(le, &p.peerReads)
	}

	// Keep the blocks the last mount read hot for eviction.
	if err := p.reads.load(ctx, m.wb); err != nil {
		le.WithError(err).Warn("unable to load the saved block reads, eviction keeps only new reads")
	}

	// Classify the blocks peers serve the placement until the mount ends, and
	// publish its backend.
	p.startPeerClassify(ctx)
	p.setBackend(m.backend)
	return p
}

// recordedReads returns the local store as the placement's reads see it:
// each hit counted, each read recorded for eviction unless in serve mode, and
// the linked files read after a local miss, as local hits.
func (p *placementState) recordedReads() block.StoreOps {
	var recorded block.StoreOps = &hitCounter{StoreOps: p.local, hits: &p.localHits}
	if !p.serveOnly {
		recorded = &readRecorder{StoreOps: recorded, reads: &p.reads}
	}
	return &fallbackReads{StoreOps: recorded, fallback: p.fileSegments}
}

// live reports which blocks a bucket reclaim must keep: the blocks the volume
// keeps child lists for, and any the local store holds. Only the collector
// ends a listed block's life, so a block whose local value was dropped stays
// live, and a volume without child lists keeps what it stores.
func (p *placementState) live(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Keep the blocks the graph tracks.
	live := make([]bool, len(refs))
	if p.tracker != nil {
		tracked, err := p.tracker.HasChildLists(ctx, refs)
		if err != nil {
			return nil, err
		}
		live = tracked
	}

	// Keep the untracked blocks the local store holds.
	var rest []*block.BlockRef
	var restAt []int
	for i, ok := range live {
		if !ok {
			rest = append(rest, refs[i])
			restAt = append(restAt, i)
		}
	}
	if len(rest) == 0 {
		return live, nil
	}
	held, err := p.local.GetBlockExistsBatch(ctx, rest)
	if err != nil {
		return nil, err
	}
	for j, ok := range held {
		live[restAt[j]] = ok
	}
	return live, nil
}

// fill writes a block read from the backend or a peer into the local store. A
// block the graph tracks lost only its value to eviction, so it is refilled
// below the ownership layer; any other block is a cache fill.
func (p *placementState) fill(ctx context.Context, primary block.StoreOps, ref *block.BlockRef, stored *block.StoredBlock) error {
	// Serve mode writes nothing.
	if p.serveOnly {
		return nil
	}

	// Refill a tracked block of a placed store.
	if p.backend.Load() != nil && p.tracker != nil {
		tracked, err := p.tracker.HasChildLists(ctx, []*block.BlockRef{ref})
		if err != nil {
			return err
		}

		// Account for every cached value, including a block whose graph was
		// omitted during migration. An untracked fill also records child refs
		// without taking ownership; eviction checks the target before dropping
		// either value, including a fill served by a peer. Both queue behind
		// the volume's writers.
		if tracked[0] {
			p.wb.Refill(ref, stored)
		} else {
			p.wb.CacheFill(ref, stored)
		}

		// Keep a remote hit hot for the same interval as a local hit.
		p.reads.add(ref.GetHash())
		return nil
	}

	// Fill any other block as a cache fill.
	return block_store.CacheFill(ctx, primary, ref, stored)
}

// uploadedBefore reports whether the storage backend holds every block written
// before at: the queued writes and the packs of the store's packahead log.
func (p *placementState) uploadedBefore(ctx context.Context, at time.Time) (bool, error) {
	uploaded, err := p.wb.UploadedBefore(ctx, at)
	if err != nil || !uploaded || p.drain == nil {
		return uploaded, err
	}
	return p.drain.UploadedBefore(at), nil
}
