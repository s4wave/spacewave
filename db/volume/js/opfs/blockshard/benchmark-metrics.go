//go:build js

package blockshard

import (
	"sync"
	"time"
)

// BenchmarkMetrics records blockshard write accounting for an explicitly
// instrumented benchmark engine. A nil recorder disables all accounting.
type BenchmarkMetrics struct {
	mu       sync.Mutex
	snapshot BenchmarkMetricsSnapshot
}

// BenchmarkMetricsSnapshot is one point-in-time copy of benchmark accounting.
type BenchmarkMetricsSnapshot struct {
	AcceptedRequests    int64
	AcceptedEntries     int64
	AcceptedBytes       int64
	ActorCycles         int64
	DrainRounds         int64
	PublishAttempts     int64
	PublishSuccesses    int64
	PublishErrors       int64
	PublishErrorEntries int64
	PublishedEntries    int64
	PublishedBytes      int64
	PublishedSegments   int64
	ManifestSlotReads   int64
	ManifestWrites      int64
	ReclaimCalls        int64
	ReclaimHits         int64
	ReclaimDeletes      int64
	ReclaimNanoseconds  int64
}

// Reset clears accounting after benchmark setup.
func (m *BenchmarkMetrics) Reset() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.snapshot = BenchmarkMetricsSnapshot{}
	m.mu.Unlock()
}

// Snapshot returns a stable copy of the current accounting.
func (m *BenchmarkMetrics) Snapshot() BenchmarkMetricsSnapshot {
	if m == nil {
		return BenchmarkMetricsSnapshot{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshot
}

func (m *BenchmarkMetrics) update(fn func(*BenchmarkMetricsSnapshot)) {
	m.mu.Lock()
	fn(&m.snapshot)
	m.mu.Unlock()
}

func (m *BenchmarkMetrics) accept(entries, bytes int) {
	m.update(func(s *BenchmarkMetricsSnapshot) {
		s.AcceptedRequests++
		s.AcceptedEntries += int64(entries)
		s.AcceptedBytes += int64(bytes)
	})
}

func (m *BenchmarkMetrics) actorCycle() {
	m.update(func(s *BenchmarkMetricsSnapshot) { s.ActorCycles++ })
}

func (m *BenchmarkMetrics) drainRound() {
	m.update(func(s *BenchmarkMetricsSnapshot) { s.DrainRounds++ })
}

func (m *BenchmarkMetrics) publish(entries, bytes, segments int, err error) {
	m.update(func(s *BenchmarkMetricsSnapshot) {
		s.PublishAttempts++
		if err != nil {
			s.PublishErrors++
			s.PublishErrorEntries += int64(entries)
			return
		}
		s.PublishSuccesses++
		s.PublishedEntries += int64(entries)
		s.PublishedBytes += int64(bytes)
		s.PublishedSegments += int64(segments)
	})
}

func (m *BenchmarkMetrics) manifestSlotRead() {
	m.update(func(s *BenchmarkMetricsSnapshot) { s.ManifestSlotReads++ })
}

func (m *BenchmarkMetrics) manifestWrite() {
	m.update(func(s *BenchmarkMetricsSnapshot) { s.ManifestWrites++ })
}

func (m *BenchmarkMetrics) reclaim(hit bool, deletes int, elapsed time.Duration) {
	m.update(func(s *BenchmarkMetricsSnapshot) {
		s.ReclaimCalls++
		if hit {
			s.ReclaimHits++
		}
		s.ReclaimDeletes += int64(deletes)
		s.ReclaimNanoseconds += elapsed.Nanoseconds()
	})
}
