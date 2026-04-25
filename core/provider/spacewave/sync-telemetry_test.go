package provider_spacewave

import (
	"testing"
	"time"

	"github.com/pkg/errors"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
)

type testSyncTelemetryFetchStats struct {
	stats packfile_store.PackfileStoreStats
}

func (s *testSyncTelemetryFetchStats) SnapshotStats() packfile_store.PackfileStoreStats {
	return s.stats
}

func TestSyncTelemetrySnapshotTransitions(t *testing.T) {
	t.Parallel()

	fetchAt := time.Unix(10, 0)
	fetchStats := &testSyncTelemetryFetchStats{
		stats: packfile_store.PackfileStoreStats{
			InFlightFetches:           1,
			FetchCount:                2,
			FetchedBytes:              2048,
			LastFetchAt:               fetchAt,
			LastFetchBytes:            1024,
			RangeRequestCount:         3,
			RangeResponseBytes:        3072,
			FullResponseFallbackCount: 1,
			FullResponseFallbackBytes: 16,
			LastFullResponseFallback:  16,
			ManifestEntries:           2,
			PackBlockCountTotal:       12,
			PackBlockCountMin:         2,
			PackBlockCountMax:         10,
			PackSizeBytesTotal:        4096,
			PackSizeBytesMin:          1024,
			PackSizeBytesMax:          3072,
			BloomFilterCount:          2,
			BloomParameterShapeCount:  1,
			BloomMaxFalsePositiveRate: 0.02,
			BloomRiskPackCount:        1,
			LookupCount:               4,
			CandidatePacks:            9,
			OpenedPacks:               5,
			NegativePacks:             3,
			TargetHits:                1,
			LastCandidatePacks:        3,
			LastOpenedPacks:           2,
			LastNegativePacks:         1,
			LastTargetHit:             true,
			IndexCacheHits:            6,
			IndexCacheMisses:          7,
			IndexCacheReadErrors:      1,
			IndexCacheWriteErrors:     2,
			RemoteIndexLoads:          8,
			RemoteIndexBytes:          8192,
			LastRemoteIndexBytes:      512,
		},
	}
	acc := &ProviderAccount{}
	release := acc.registerSyncTelemetryStore("bstore-1", fetchStats)
	defer release()

	snap := acc.GetSyncTelemetrySnapshot()
	if snap.StoreCount != 1 {
		t.Fatalf("store count = %d, want 1", snap.StoreCount)
	}
	if snap.InFlightFetches != 1 || snap.FetchCount != 2 || snap.FetchedBytes != 2048 {
		t.Fatalf("unexpected fetch counters: %+v", snap)
	}
	if !snap.LastFetchAt.Equal(fetchAt) || !snap.LastActivityAt.Equal(fetchAt) {
		t.Fatalf("unexpected fetch activity time: %+v", snap)
	}
	if snap.RangeRequestCount != 3 || snap.RangeResponseBytes != 3072 {
		t.Fatalf("unexpected range counters: %+v", snap)
	}
	if snap.ManifestEntries != 2 ||
		snap.PackBlockCountTotal != 12 ||
		snap.PackBlockCountMin != 2 ||
		snap.PackBlockCountMax != 10 {
		t.Fatalf("unexpected manifest distribution counters: %+v", snap)
	}
	if snap.BloomMaxFalsePositiveRate != 0.02 || snap.BloomRiskPackCount != 1 {
		t.Fatalf("unexpected bloom risk counters: %+v", snap)
	}
	if snap.LookupCount != 4 ||
		snap.CandidatePacks != 9 ||
		snap.OpenedPacks != 5 ||
		snap.NegativePacks != 3 ||
		snap.TargetHits != 1 ||
		!snap.LastTargetHit {
		t.Fatalf("unexpected lookup counters: %+v", snap)
	}
	if snap.IndexCacheHits != 6 ||
		snap.IndexCacheMisses != 7 ||
		snap.RemoteIndexLoads != 8 ||
		snap.RemoteIndexBytes != 8192 {
		t.Fatalf("unexpected index counters: %+v", snap)
	}

	acc.addSyncTelemetryDirty("bstore-1", 512)
	snap = acc.GetSyncTelemetrySnapshot()
	if snap.UploadPhase != SyncTelemetryUploadPhaseDirtyPending {
		t.Fatalf("upload phase = %v, want dirty pending", snap.UploadPhase)
	}
	if snap.PendingUploadBytes != 512 || snap.PendingUploadCount != 1 {
		t.Fatalf("unexpected dirty backlog: %+v", snap)
	}

	acc.startSyncTelemetryPush("bstore-1", 128)
	snap = acc.GetSyncTelemetrySnapshot()
	if snap.UploadPhase != SyncTelemetryUploadPhasePushing {
		t.Fatalf("upload phase = %v, want pushing", snap.UploadPhase)
	}
	if snap.InFlightPushes != 1 || snap.ActiveUploadBytes != 128 {
		t.Fatalf("unexpected active push state: %+v", snap)
	}
	acc.setSyncTelemetryPushProgress("bstore-1", 64)
	snap = acc.GetSyncTelemetrySnapshot()
	if snap.ActiveUploadTransferredBytes != 64 {
		t.Fatalf("active upload sent = %d, want 64", snap.ActiveUploadTransferredBytes)
	}

	acc.finishSyncTelemetryPush("bstore-1", 128, nil)
	snap = acc.GetSyncTelemetrySnapshot()
	if snap.InFlightPushes != 0 ||
		snap.ActiveUploadBytes != 0 ||
		snap.ActiveUploadTransferredBytes != 0 {
		t.Fatalf("push remained active: %+v", snap)
	}
	if snap.PushCount != 1 || snap.PushedBytes != 128 || snap.LastPushAt.IsZero() {
		t.Fatalf("unexpected completed push counters: %+v", snap)
	}
	if snap.LastPushError != "" || snap.LastError != "" {
		t.Fatalf("unexpected push error after success: %+v", snap)
	}

	acc.startSyncTelemetryPull("bstore-1")
	snap = acc.GetSyncTelemetrySnapshot()
	if snap.PullActiveCount != 1 {
		t.Fatalf("pull active count = %d, want 1", snap.PullActiveCount)
	}
	pullErr := errors.New("pull failed")
	acc.finishSyncTelemetryPull("bstore-1", pullErr)
	snap = acc.GetSyncTelemetrySnapshot()
	if snap.PullActiveCount != 0 || snap.LastPullError != pullErr.Error() {
		t.Fatalf("unexpected pull error state: %+v", snap)
	}
	if snap.LastError != pullErr.Error() {
		t.Fatalf("last error = %q, want pull error", snap.LastError)
	}

	pushErr := errors.New("push failed")
	acc.startSyncTelemetryPush("bstore-1", 64)
	acc.finishSyncTelemetryPush("bstore-1", 64, pushErr)
	snap = acc.GetSyncTelemetrySnapshot()
	if snap.UploadPhase != SyncTelemetryUploadPhaseError {
		t.Fatalf("upload phase = %v, want error", snap.UploadPhase)
	}
	if snap.LastPushError != pushErr.Error() || snap.LastError != pushErr.Error() {
		t.Fatalf("unexpected push error state: %+v", snap)
	}
}

func TestSyncTelemetryStoreStatsChangeBroadcast(t *testing.T) {
	t.Parallel()

	acc := &ProviderAccount{}
	store := packfile_store.NewPackfileStore(nil, nil)
	_, ch := acc.GetSyncTelemetrySnapshotWithWait()
	release := acc.registerSyncTelemetryStore("bstore-1", store)
	defer release()
	waitForSyncTelemetryBroadcast(t, ch)

	_, ch = acc.GetSyncTelemetrySnapshotWithWait()
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:         "pack-1",
		BlockCount: 1,
		SizeBytes:  2,
	}})
	waitForSyncTelemetryBroadcast(t, ch)

	snap := acc.GetSyncTelemetrySnapshot()
	if snap.ManifestEntries != 1 || snap.PackBlockCountTotal != 1 || snap.PackSizeBytesTotal != 2 {
		t.Fatalf("unexpected manifest stats after callback: %+v", snap)
	}
}

func waitForSyncTelemetryBroadcast(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for sync telemetry broadcast")
	}
}

// _ is a type assertion
var _ syncTelemetryFetchStatsProvider = ((*testSyncTelemetryFetchStats)(nil))
