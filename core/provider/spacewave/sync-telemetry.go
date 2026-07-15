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

// SyncTelemetryBlockStoreSnapshot describes one mounted block store.
type SyncTelemetryBlockStoreSnapshot = synctelemetry.BlockStoreSnapshot

// SyncTelemetryBlockSource identifies the owner-observed source of a block read.
type SyncTelemetryBlockSource = synctelemetry.BlockSource

const (
	// SyncTelemetryBlockSourceUnknown means no source has been observed.
	SyncTelemetryBlockSourceUnknown = synctelemetry.BlockSourceUnknown
	// SyncTelemetryBlockSourceCache means the local cache satisfied the read.
	SyncTelemetryBlockSourceCache = synctelemetry.BlockSourceCache
	// SyncTelemetryBlockSourceDirect means demand-driven DEX satisfied the read.
	SyncTelemetryBlockSourceDirect = synctelemetry.BlockSourceDirect
	// SyncTelemetryBlockSourceCloud means the Cloud baseline satisfied the read.
	SyncTelemetryBlockSourceCloud = synctelemetry.BlockSourceCloud
)

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

func (a *ProviderAccount) recordSyncTelemetryError(bstoreID string, err error) {
	a.syncTelemetry.RecordError(bstoreID, err)
}

func (a *ProviderAccount) recordSyncTelemetryBlockSource(bstoreID string, source SyncTelemetryBlockSource) {
	a.syncTelemetry.RecordBlockSource(bstoreID, source)
}

func (a *ProviderAccount) setSyncTelemetryAcceptedRoot(bstoreID, sharedObjectID string, sequence uint64) {
	a.syncTelemetry.SetAcceptedRoot(bstoreID, sharedObjectID, sequence)
}

func (a *ProviderAccount) setSyncTelemetryCloudRemoteSequence(bstoreID string, sequence uint64) {
	a.syncTelemetry.SetCloudRemoteSequence(bstoreID, sequence)
}

// _ is a type assertion
var _ syncTelemetryFetchStatsProvider = ((*packfile_store.PackfileStore)(nil))
