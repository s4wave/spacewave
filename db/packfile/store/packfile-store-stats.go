package store

import (
	"maps"
	"time"

	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/db/packfile"
)

// PackfileStoreStats describes aggregate store state across all open engines.
type PackfileStoreStats struct {
	EngineCount               int
	ResidentBytes             int64
	SpanCount                 int
	InFlightFetches           int
	FetchCount                uint64
	FetchedBytes              int64
	RangeRequestCount         uint64
	RangeResponseBytes        int64
	IndexTailFetchCount       uint64
	IndexTailFetchBytes       int64
	IndexTailResponseBytes    int64
	FullResponseFallbackCount uint64
	FullResponseFallbackBytes int64
	LastFullResponseFallback  int64
	LastFetchAt               time.Time
	LastFetchBytes            int
	PublishedBlocks           int
	WritebackRunning          int
	VerifyFailures            uint64
	WritebackCount            uint64
	WritebackErrors           uint64
	ManifestEntries           int
	PackBlockCountTotal       uint64
	PackBlockCountMin         uint64
	PackBlockCountMax         uint64
	PackSizeBytesTotal        uint64
	PackSizeBytesMin          uint64
	PackSizeBytesMax          uint64
	BloomFilterCount          int
	BloomMissingCount         int
	BloomInvalidCount         int
	BloomMaxFalsePositiveRate float64
	BloomRiskPackCount        int
	WritebackWindow           int64
	ResidentByteBudget        int64
	LookupCount               uint64
	CandidatePacks            uint64
	OpenedPacks               uint64
	NegativePacks             uint64
	TargetHits                uint64
	LastCandidatePacks        int
	LastOpenedPacks           int
	LastNegativePacks         int
	LastTargetHit             bool
	IndexCacheHits            uint64
	IndexCacheMisses          uint64
	IndexCacheReadErrors      uint64
	IndexCacheWriteErrors     uint64
	RemoteIndexLoads          uint64
	RemoteIndexBytes          int64
	LastRemoteIndexBytes      int64
}

// packLookup counts the packs one lookup consulted.
type packLookup struct {
	// candidates is the number of packs whose bloom filter may hold the key.
	candidates int
	// opened is the number of candidate engines consulted.
	opened int
	// negative is the number of consulted engines that lacked the key.
	negative int
	// hit reports whether an engine held the key.
	hit bool
}

type packLookupStats struct {
	LookupCount        uint64
	CandidatePacks     uint64
	OpenedPacks        uint64
	NegativePacks      uint64
	TargetHits         uint64
	LastCandidatePacks int
	LastOpenedPacks    int
	LastNegativePacks  int
	LastTargetHit      bool
}

const bloomFalsePositiveRiskThreshold = 0.01

// SnapshotStats returns aggregate store state across all open engines.
func (s *PackfileStore) SnapshotStats() PackfileStoreStats {
	// Snapshot open readers, writeback configuration, and lookup counters.
	s.mtx.Lock()
	engines := s.snapshotEnginesLocked()
	writebackWindow := s.writebackWindow
	residentByteBudget := s.budget.limit.Load()
	stats := s.stats
	s.mtx.Unlock()

	// Summarize the current manifest pack and bloom distributions.
	entries := s.SnapshotManifest().GetEntries()
	manifestStats := summarizeManifestDistribution(entries)

	// Initialize aggregate stats from the manifest and store configuration.
	snap := PackfileStoreStats{
		EngineCount:               len(engines),
		ManifestEntries:           len(entries),
		PackBlockCountTotal:       manifestStats.PackBlockCountTotal,
		PackBlockCountMin:         manifestStats.PackBlockCountMin,
		PackBlockCountMax:         manifestStats.PackBlockCountMax,
		PackSizeBytesTotal:        manifestStats.PackSizeBytesTotal,
		PackSizeBytesMin:          manifestStats.PackSizeBytesMin,
		PackSizeBytesMax:          manifestStats.PackSizeBytesMax,
		BloomFilterCount:          manifestStats.BloomFilterCount,
		BloomMissingCount:         manifestStats.BloomMissingCount,
		BloomInvalidCount:         manifestStats.BloomInvalidCount,
		BloomMaxFalsePositiveRate: manifestStats.BloomMaxFalsePositiveRate,
		BloomRiskPackCount:        manifestStats.BloomRiskPackCount,
		WritebackWindow:           writebackWindow,
		ResidentByteBudget:        residentByteBudget,
		LookupCount:               stats.LookupCount,
		CandidatePacks:            stats.CandidatePacks,
		OpenedPacks:               stats.OpenedPacks,
		NegativePacks:             stats.NegativePacks,
		TargetHits:                stats.TargetHits,
		LastCandidatePacks:        stats.LastCandidatePacks,
		LastOpenedPacks:           stats.LastOpenedPacks,
		LastNegativePacks:         stats.LastNegativePacks,
		LastTargetHit:             stats.LastTargetHit,
	}

	// Combine the resident state and counters of every open pack reader.
	for _, e := range engines {
		// Accumulate resident bytes and payload transport totals.
		es := e.SnapshotStats()
		snap.ResidentBytes += es.ResidentBytes
		snap.SpanCount += es.SpanCount
		snap.InFlightFetches += es.InFlightFetches
		snap.FetchCount += es.FetchCount
		snap.FetchedBytes += es.FetchedBytes
		snap.RangeRequestCount += es.RangeRequestCount
		snap.RangeResponseBytes += es.RangeResponseBytes

		// Accumulate index-tail and full-response transport totals.
		snap.IndexTailFetchCount += es.IndexTailFetchCount
		snap.IndexTailFetchBytes += es.IndexTailFetchBytes
		snap.IndexTailResponseBytes += es.IndexTailResponseBytes
		snap.FullResponseFallbackCount += es.FullResponseFallbackCount
		snap.FullResponseFallbackBytes += es.FullResponseFallbackBytes
		if snap.LastFullResponseFallback < es.LastFullResponseFallback {
			snap.LastFullResponseFallback = es.LastFullResponseFallback
		}

		// Retain the latest completed transport fetch details.
		if snap.LastFetchAt.Before(es.LastFetchAt) {
			snap.LastFetchAt = es.LastFetchAt
			snap.LastFetchBytes = es.LastFetchBytes
		}

		// Accumulate writeback publication and verification counters.
		snap.PublishedBlocks += es.PublishedBlocks
		snap.WritebackRunning += es.WritebackRunning
		snap.VerifyFailures += es.VerifyFailures
		snap.WritebackCount += es.WritebackCount
		snap.WritebackErrors += es.WritebackErrors

		// Accumulate index cache and remote index load totals.
		snap.IndexCacheHits += es.IndexCacheHits
		snap.IndexCacheMisses += es.IndexCacheMisses
		snap.IndexCacheReadErrors += es.IndexCacheReadErrors
		snap.IndexCacheWriteErrors += es.IndexCacheWriteErrors
		snap.RemoteIndexLoads += es.RemoteIndexLoads
		snap.RemoteIndexBytes += es.RemoteIndexBytes
		if snap.LastRemoteIndexBytes < es.LastRemoteIndexBytes {
			snap.LastRemoteIndexBytes = es.LastRemoteIndexBytes
		}
	}
	return snap
}

func summarizeManifestDistribution(entries []*packfile.PackfileEntry) PackfileStoreStats {
	// Return an empty distribution when the manifest contains no packs.
	stats := PackfileStoreStats{}
	if len(entries) == 0 {
		return stats
	}

	// Summarize pack sizes, block counts, and bloom filter risk.
	for i, entry := range entries {
		// Accumulate the pack block count distribution.
		blockCount := entry.GetBlockCount()
		sizeBytes := entry.GetSizeBytes()
		stats.PackBlockCountTotal += blockCount
		stats.PackSizeBytesTotal += sizeBytes
		if i == 0 || blockCount < stats.PackBlockCountMin {
			stats.PackBlockCountMin = blockCount
		}
		if stats.PackBlockCountMax < blockCount {
			stats.PackBlockCountMax = blockCount
		}

		// Retain the smallest and largest manifest pack sizes.
		if i == 0 || sizeBytes < stats.PackSizeBytesMin {
			stats.PackSizeBytesMin = sizeBytes
		}
		if stats.PackSizeBytesMax < sizeBytes {
			stats.PackSizeBytesMax = sizeBytes
		}

		// Count missing and invalid pack bloom filters.
		bf := manifestEntryBloomFilter(entry)
		if bf == nil {
			if len(entry.GetBloomFilter()) == 0 {
				stats.BloomMissingCount++
				continue
			}
			stats.BloomInvalidCount++
			continue
		}

		// Estimate false-positive risk for each valid pack bloom filter.
		stats.BloomFilterCount++
		fp := bloom.EstimateFalsePositiveRate(bf.Cap(), bf.K(), uint(blockCount))
		if stats.BloomMaxFalsePositiveRate < fp {
			stats.BloomMaxFalsePositiveRate = fp
		}
		if fp > bloomFalsePositiveRiskThreshold {
			stats.BloomRiskPackCount++
		}
	}
	return stats
}

func manifestEntryBloomFilter(entry *packfile.PackfileEntry) *bloom.Filter {
	// Require encoded bloom bytes before decoding the pack filter.
	bloomData := entry.GetBloomFilter()
	if len(bloomData) == 0 {
		return nil
	}

	// Decode the manifest bloom record into its lookup filter.
	var pbf bloom.BloomFilter
	if err := pbf.UnmarshalBlock(bloomData); err != nil {
		return nil
	}
	return pbf.ToBloomFilter()
}

// SnapshotEngineStats returns per-engine stats keyed by manifest pack id.
func (s *PackfileStore) SnapshotEngineStats() map[string]PackReaderStats {
	// Snapshot the reader registry before collecting per-pack stats.
	s.mtx.Lock()
	engines := make(map[string]*PackReader, len(s.engines))
	maps.Copy(engines, s.engines)
	s.mtx.Unlock()

	// Collect stats from the retained pack readers.
	stats := make(map[string]PackReaderStats, len(engines))
	for id, e := range engines {
		stats[id] = e.SnapshotStats()
	}
	return stats
}
