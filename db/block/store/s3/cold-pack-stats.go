//go:build !tinygo

package block_store_s3

import "sync/atomic"

// ColdPackStats reports cold reads and log progress on the admin socket.
type ColdPackStats struct {
	// Ready reports whether the manifest has been published.
	Ready bool
	// LoadedPacks counts packs loaded in the immutable manifest.
	LoadedPacks int
	// GonePackIDs names packs fenced after an authoritative not-found response.
	GonePackIDs []string
	// UncachedExistenceMisses counts false answers with at least one uncached candidate.
	UncachedExistenceMisses uint64
	// BloomProbes counts filters consulted after interval selection.
	BloomProbes uint64
	// BloomHits counts positive filter probes.
	BloomHits uint64
	// TailFetches counts range requests for tails, including background fills.
	TailFetches uint64
	// FalsePositives counts bloom hits rejected by an exact tail lookup.
	FalsePositives uint64
	// TrueHits counts successful cold lookups.
	TrueHits uint64
	// PromotedKeys counts durable records recovered or written in this mount.
	PromotedKeys uint64
	// PromoteDropped counts offers dropped by a full promotion queue.
	PromoteDropped uint64
	// PromoteCapDropped counts offers rejected by the promoted record limit.
	PromoteCapDropped uint64
	// TailFillDropped counts offers dropped by a full background tail queue.
	TailFillDropped uint64
	// LastNewKeyUnix is the Unix time of the latest durable append in this mount.
	LastNewKeyUnix int64
}

// coldPackCounters records the tier's evidence without request-path reporting I/O.
type coldPackCounters struct {
	// uncachedExistenceMisses counts misses that queued a pack tail.
	uncachedExistenceMisses atomic.Uint64
	// bloomProbes counts filters consulted.
	bloomProbes atomic.Uint64
	// bloomHits counts positive filter probes.
	bloomHits atomic.Uint64
	// tailFetches counts tail range reads.
	tailFetches atomic.Uint64
	// falsePositives counts bloom positives disproved by exact tails.
	falsePositives atomic.Uint64
	// trueHits counts successful block, stat and existence lookups.
	trueHits atomic.Uint64
	// promotedKeys counts the durable log prefix in records.
	promotedKeys atomic.Uint64
	// promoteDropped counts full-queue drops.
	promoteDropped atomic.Uint64
	// promoteCapDropped counts full-log drops.
	promoteCapDropped atomic.Uint64
	// tailFillDropped counts full-queue tail requests.
	tailFillDropped atomic.Uint64
	// lastNewKeyUnix marks the last successful append batch.
	lastNewKeyUnix atomic.Int64
}

// Stats reads the current counters without waiting for preload or a worker.
func (s *ColdPackStore) Stats() ColdPackStats {
	// Read counters without waiting for a cache fill or log write.
	catalog := s.catalog.Load()
	stats := ColdPackStats{
		Ready:                   catalog != nil,
		UncachedExistenceMisses: s.stats.uncachedExistenceMisses.Load(),
		BloomProbes:             s.stats.bloomProbes.Load(),
		BloomHits:               s.stats.bloomHits.Load(),
		TailFetches:             s.stats.tailFetches.Load(),
		FalsePositives:          s.stats.falsePositives.Load(),
		TrueHits:                s.stats.trueHits.Load(),
		PromotedKeys:            s.stats.promotedKeys.Load(),
		PromoteDropped:          s.stats.promoteDropped.Load(),
		PromoteCapDropped:       s.stats.promoteCapDropped.Load(),
		TailFillDropped:         s.stats.tailFillDropped.Load(),
		LastNewKeyUnix:          s.stats.lastNewKeyUnix.Load(),
	}

	// This admin-only scan names packs that no longer authorize a positive.
	if catalog != nil {
		stats.LoadedPacks = len(catalog.packs)
		for i := range catalog.packs {
			if pack := &catalog.packs[i]; pack.gone.Load() {
				stats.GonePackIDs = append(stats.GonePackIDs, pack.meta.Entry.Id)
			}
		}
	}
	return stats
}
