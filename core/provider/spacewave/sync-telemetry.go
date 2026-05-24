package provider_spacewave

import (
	"github.com/aperturerobotics/util/broadcast"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
	"github.com/s4wave/spacewave/core/provider/spacewave/synctelemetry"
)

// SyncTelemetryUploadPhase describes upload-side Spacewave sync activity.
type SyncTelemetryUploadPhase = synctelemetry.UploadPhase

const (
	// SyncTelemetryUploadPhaseIdle means no upload work is known.
	SyncTelemetryUploadPhaseIdle = synctelemetry.UploadPhaseIdle
	// SyncTelemetryUploadPhaseDirtyPending means dirty blocks are waiting to upload.
	SyncTelemetryUploadPhaseDirtyPending = synctelemetry.UploadPhaseDirtyPending
	// SyncTelemetryUploadPhasePushing means a packfile push is in flight.
	SyncTelemetryUploadPhasePushing = synctelemetry.UploadPhasePushing
	// SyncTelemetryUploadPhaseError means upload sync has a recorded error.
	SyncTelemetryUploadPhaseError = synctelemetry.UploadPhaseError
)

// SyncTelemetrySnapshot describes Spacewave cloud sync activity.
type SyncTelemetrySnapshot = synctelemetry.Snapshot

type syncTelemetryFetchStatsProvider = synctelemetry.FetchStatsProvider

// GetSyncTelemetryBroadcast returns the broadcast guarding Spacewave sync telemetry.
func (a *ProviderAccount) GetSyncTelemetryBroadcast() *broadcast.Broadcast {
	return a.syncTelemetry.Broadcast()
}

// GetSyncTelemetrySnapshot returns aggregate Spacewave sync telemetry.
func (a *ProviderAccount) GetSyncTelemetrySnapshot() SyncTelemetrySnapshot {
	return a.syncTelemetry.Snapshot()
}

// GetSyncTelemetrySnapshotWithWait returns aggregate sync telemetry and its wait channel.
func (a *ProviderAccount) GetSyncTelemetrySnapshotWithWait() (SyncTelemetrySnapshot, <-chan struct{}) {
	return a.syncTelemetry.SnapshotWithWait()
}

func (a *ProviderAccount) registerSyncTelemetryStore(
	bstoreID string,
	fetchStats syncTelemetryFetchStatsProvider,
) func() {
	return a.syncTelemetry.RegisterStore(bstoreID, fetchStats)
}

func (a *ProviderAccount) setSyncTelemetryPending(bstoreID string, bytes int64, count int) {
	a.syncTelemetry.SetPending(bstoreID, bytes, count)
}

func (a *ProviderAccount) addSyncTelemetryDirty(bstoreID string, bytes int64) {
	a.syncTelemetry.AddDirty(bstoreID, bytes)
}

func (a *ProviderAccount) startSyncTelemetryPush(bstoreID string, bytes int64) {
	a.syncTelemetry.StartPush(bstoreID, bytes)
}

func (a *ProviderAccount) setSyncTelemetryPushProgress(bstoreID string, bytes int64) {
	a.syncTelemetry.SetPushProgress(bstoreID, bytes)
}

func (a *ProviderAccount) finishSyncTelemetryPush(bstoreID string, bytes int64, err error) {
	a.syncTelemetry.FinishPush(bstoreID, bytes, err)
}

func (a *ProviderAccount) addSyncTelemetryDeduped(bstoreID string, bytes int64, count int) {
	a.syncTelemetry.AddDeduped(bstoreID, bytes, count)
}

func (a *ProviderAccount) startSyncTelemetryPull(bstoreID string) {
	a.syncTelemetry.StartPull(bstoreID)
}

func (a *ProviderAccount) finishSyncTelemetryPull(bstoreID string, err error) {
	a.syncTelemetry.FinishPull(bstoreID, err)
}

// _ is a type assertion
var _ syncTelemetryFetchStatsProvider = ((*packfile_store.PackfileStore)(nil))
