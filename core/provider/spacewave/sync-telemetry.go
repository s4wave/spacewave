package provider_spacewave

import (
	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/core/provider/spacewave/synctelemetry"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
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
	// SyncTelemetryUploadPhaseCorrupt means dirty blocks fail their hash and
	// stay queued while the rest upload.
	SyncTelemetryUploadPhaseCorrupt = synctelemetry.UploadPhaseCorrupt
)

// SyncTelemetryUploadHold identifies the account limit that holds uploads.
type SyncTelemetryUploadHold = synctelemetry.UploadHold

const (
	// SyncTelemetryUploadHoldNone means no account limit holds the uploads.
	SyncTelemetryUploadHoldNone = synctelemetry.UploadHoldNone
	// SyncTelemetryUploadHoldStorageFull means the account's storage is full.
	SyncTelemetryUploadHoldStorageFull = synctelemetry.UploadHoldStorageFull
	// SyncTelemetryUploadHoldSpendingLimit means the account reached its
	// spending limit.
	SyncTelemetryUploadHoldSpendingLimit = synctelemetry.UploadHoldSpendingLimit
)

// SyncTelemetrySnapshot describes Spacewave cloud sync activity.
type SyncTelemetrySnapshot = synctelemetry.Snapshot

// SyncTelemetryBlockStoreSnapshot describes one mounted block store.
type SyncTelemetryBlockStoreSnapshot = synctelemetry.BlockStoreSnapshot

// SyncTelemetryBlockSource identifies the observed source of a block read.
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

// _ is a type assertion
var _ syncTelemetryFetchStatsProvider = (*packfile_store.PackfileStore)(nil)
