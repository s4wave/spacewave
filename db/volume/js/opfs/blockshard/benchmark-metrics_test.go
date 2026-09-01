//go:build js

package blockshard

import (
	"errors"
	"testing"
	"time"
)

func TestBenchmarkMetricsSnapshotResetAndReconciliation(t *testing.T) {
	metrics := &BenchmarkMetrics{}
	metrics.accept(3, 30)
	metrics.actorCycle()
	metrics.drainRound()
	metrics.publish(2, 20, 1, nil)
	metrics.publish(1, 10, 1, errors.New("publish failed"))
	metrics.manifestSlotRead()
	metrics.manifestSlotRead()
	metrics.manifestWrite()
	metrics.reclaim(false, 0, 2*time.Millisecond)

	snapshot := metrics.Snapshot()
	if snapshot.AcceptedEntries != snapshot.PublishedEntries+snapshot.PublishErrorEntries {
		t.Fatalf("accepted entries do not reconcile: %+v", snapshot)
	}
	if snapshot.PublishAttempts != snapshot.PublishSuccesses+snapshot.PublishErrors {
		t.Fatalf("publish attempts do not reconcile: %+v", snapshot)
	}
	if snapshot.ManifestSlotReads != 2 || snapshot.ManifestWrites != 1 {
		t.Fatalf("manifest accounting differs: %+v", snapshot)
	}
	if snapshot.ReclaimCalls != 1 || snapshot.ReclaimHits != 0 ||
		snapshot.ReclaimNanoseconds != int64(2*time.Millisecond) {
		t.Fatalf("reclaim accounting differs: %+v", snapshot)
	}

	metrics.Reset()
	if reset := metrics.Snapshot(); reset != (BenchmarkMetricsSnapshot{}) {
		t.Fatalf("reset snapshot is not empty: %+v", reset)
	}
}

func TestNilBenchmarkMetricsSnapshot(t *testing.T) {
	var metrics *BenchmarkMetrics
	if snapshot := metrics.Snapshot(); snapshot != (BenchmarkMetricsSnapshot{}) {
		t.Fatalf("nil snapshot is not empty: %+v", snapshot)
	}
	metrics.Reset()
}
