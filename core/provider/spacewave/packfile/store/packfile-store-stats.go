package store

import (
	"maps"
	"time"

	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/db/block/bloom"
)

// PackfileStoreStats describes aggregate store state across all open engines.
type PackfileStoreStats struct {
	EngineCount               int
	ResidentBytes             int64
	PinnedBytes               int64
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
	BlockCount                int
	VerifyingBlocks           int
	VerifiedBlocks            int
	PublishedBlocks           int
	FailedBlocks              int
	VerifyQueued              int
	VerifyRunning             int
	VerifyCompleted           uint64
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
	BloomParameterShapeCount  int
	BloomMaxFalsePositiveRate float64
	BloomRiskPackCount        int
	WritebackWindow           int64
	ResidentByteBudget        int64
	IndexPromotionSet         bool
	IndexPromotionValue       bool
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

type bloomParameterShape struct {
	m uint
	k uint
}

const bloomFalsePositiveRiskThreshold = 0.01

// SnapshotStats returns aggregate store state across all open engines.
func (s *PackfileStore) SnapshotStats() PackfileStoreStats {
	s.mu.Lock()
	engines := s.snapshotEnginesLocked()
	writebackWindow := s.writebackWindow
	residentByteBudget := s.maxBytes
	indexPromotionSet := s.tuningOverrides.indexPromotionSet
	indexPromotionValue := s.tuningOverrides.indexPromotion
	stats := s.stats
	s.mu.Unlock()

	var entries []*packfile.PackfileEntry
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		entries = s.manifest
	})
	manifestStats := summarizeManifestDistribution(entries)

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
		BloomParameterShapeCount:  manifestStats.BloomParameterShapeCount,
		BloomMaxFalsePositiveRate: manifestStats.BloomMaxFalsePositiveRate,
		BloomRiskPackCount:        manifestStats.BloomRiskPackCount,
		WritebackWindow:           writebackWindow,
		ResidentByteBudget:        residentByteBudget,
		IndexPromotionSet:         indexPromotionSet,
		IndexPromotionValue:       indexPromotionValue,
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
	for _, e := range engines {
		es := e.SnapshotStats()
		snap.ResidentBytes += es.ResidentBytes
		snap.PinnedBytes += es.PinnedBytes
		snap.SpanCount += es.SpanCount
		snap.InFlightFetches += es.InFlightFetches
		snap.FetchCount += es.FetchCount
		snap.FetchedBytes += es.FetchedBytes
		snap.RangeRequestCount += es.RangeRequestCount
		snap.RangeResponseBytes += es.RangeResponseBytes
		snap.IndexTailFetchCount += es.IndexTailFetchCount
		snap.IndexTailFetchBytes += es.IndexTailFetchBytes
		snap.IndexTailResponseBytes += es.IndexTailResponseBytes
		snap.FullResponseFallbackCount += es.FullResponseFallbackCount
		snap.FullResponseFallbackBytes += es.FullResponseFallbackBytes
		if snap.LastFullResponseFallback < es.LastFullResponseFallback {
			snap.LastFullResponseFallback = es.LastFullResponseFallback
		}
		if snap.LastFetchAt.Before(es.LastFetchAt) {
			snap.LastFetchAt = es.LastFetchAt
			snap.LastFetchBytes = es.LastFetchBytes
		}
		snap.BlockCount += es.BlockCount
		snap.VerifyingBlocks += es.VerifyingBlocks
		snap.VerifiedBlocks += es.VerifiedBlocks
		snap.PublishedBlocks += es.PublishedBlocks
		snap.FailedBlocks += es.FailedBlocks
		snap.VerifyQueued += es.VerifyQueued
		snap.VerifyRunning += es.VerifyRunning
		snap.VerifyCompleted += es.VerifyCompleted
		snap.VerifyFailures += es.VerifyFailures
		snap.WritebackCount += es.WritebackCount
		snap.WritebackErrors += es.WritebackErrors
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
	stats := PackfileStoreStats{}
	if len(entries) == 0 {
		return stats
	}
	shapes := make(map[bloomParameterShape]struct{})
	for i, entry := range entries {
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
		if i == 0 || sizeBytes < stats.PackSizeBytesMin {
			stats.PackSizeBytesMin = sizeBytes
		}
		if stats.PackSizeBytesMax < sizeBytes {
			stats.PackSizeBytesMax = sizeBytes
		}
		bf := manifestEntryBloomFilter(entry)
		if bf == nil {
			if len(entry.GetBloomFilter()) == 0 {
				stats.BloomMissingCount++
				continue
			}
			stats.BloomInvalidCount++
			continue
		}
		stats.BloomFilterCount++
		shapes[bloomParameterShape{m: bf.Cap(), k: bf.K()}] = struct{}{}
		fp := bloom.EstimateFalsePositiveRate(bf.Cap(), bf.K(), uint(blockCount))
		if stats.BloomMaxFalsePositiveRate < fp {
			stats.BloomMaxFalsePositiveRate = fp
		}
		if fp > bloomFalsePositiveRiskThreshold {
			stats.BloomRiskPackCount++
		}
	}
	stats.BloomParameterShapeCount = len(shapes)
	return stats
}

func manifestEntryBloomFilter(entry *packfile.PackfileEntry) *bloom.Filter {
	bloomData := entry.GetBloomFilter()
	if len(bloomData) == 0 {
		return nil
	}
	var pbf bloom.BloomFilter
	if err := pbf.UnmarshalBlock(bloomData); err != nil {
		return nil
	}
	return pbf.ToBloomFilter()
}

// SnapshotEngineStats returns per-engine stats keyed by manifest pack id.
func (s *PackfileStore) SnapshotEngineStats() map[string]PackReaderStats {
	s.mu.Lock()
	engines := make(map[string]*PackReader, len(s.engines))
	maps.Copy(engines, s.engines)
	s.mu.Unlock()

	stats := make(map[string]PackReaderStats, len(engines))
	for id, e := range engines {
		stats[id] = e.SnapshotStats()
	}
	return stats
}
