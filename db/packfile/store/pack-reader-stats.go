package store

import "time"

// PackReaderStats describes the current engine state and counters.
type PackReaderStats struct {
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
	CurrentWindow             int
	PublishedBlocks           int
	WritebackRunning          int
	VerifyFailures            uint64
	WritebackCount            uint64
	WritebackErrors           uint64
	IndexLoaded               bool
	IndexCacheHits            uint64
	IndexCacheMisses          uint64
	IndexCacheReadErrors      uint64
	IndexCacheWriteErrors     uint64
	RemoteIndexLoads          uint64
	RemoteIndexBytes          int64
	LastRemoteIndexBytes      int64
}

// TransportStats describes transport-specific fetch counters.
type TransportStats struct {
	FullResponseFallbackCount uint64
	FullResponseFallbackBytes int64
	LastFullResponseFallback  int64
}

type transportStatsProvider interface {
	SnapshotTransportStats() TransportStats
}

// SnapshotStats returns a snapshot of engine state and counters.
func (e *PackReader) SnapshotStats() PackReaderStats {
	var snap PackReaderStats
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		snap.ResidentBytes = e.residentBytes
		snap.SpanCount = len(e.spans)
		snap.InFlightFetches = len(e.loading)
		snap.FetchCount = e.fetchCount
		snap.FetchedBytes = e.fetchBytes
		snap.RangeRequestCount = e.fetchCount
		snap.RangeResponseBytes = e.rangeResponseBytes
		snap.IndexTailFetchCount = e.indexTailFetchCount
		snap.IndexTailFetchBytes = e.indexTailFetchBytes
		snap.IndexTailResponseBytes = e.indexTailResponseBytes
		if provider, ok := e.transport.(transportStatsProvider); ok {
			transport := provider.SnapshotTransportStats()
			snap.FullResponseFallbackCount = transport.FullResponseFallbackCount
			snap.FullResponseFallbackBytes = transport.FullResponseFallbackBytes
			snap.LastFullResponseFallback = transport.LastFullResponseFallback
		}
		snap.LastFetchAt = e.lastFetchAt
		snap.LastFetchBytes = e.lastFetchBytes
		snap.CurrentWindow = e.currentWindow
		snap.PublishedBlocks = len(e.published)
		snap.WritebackRunning = e.writebackRunning
		snap.VerifyFailures = e.verifyFailures
		snap.WritebackCount = e.writebackCount
		snap.WritebackErrors = e.writebackErrors
		snap.IndexLoaded = e.indexLoaded
		snap.IndexCacheHits = e.indexCacheHits
		snap.IndexCacheMisses = e.indexCacheMisses
		snap.IndexCacheReadErrors = e.indexCacheReadErrors
		snap.IndexCacheWriteErrors = e.indexCacheWriteErrors
		snap.RemoteIndexLoads = e.remoteIndexLoads
		snap.RemoteIndexBytes = e.remoteIndexBytes
		snap.LastRemoteIndexBytes = e.lastRemoteIndexBytes
	})
	return snap
}
