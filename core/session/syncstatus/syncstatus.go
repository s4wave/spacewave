// Package session_syncstatus projects a Session's sync and network activity.
package session_syncstatus

import (
	"context"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/s4wave/spacewave/core/pairing"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/broadcast"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

const syncStatusRateWindow = time.Second

type syncStatusCounters struct {
	uploadBytes   int64
	downloadBytes int64
}

type syncStatusRateState struct {
	lastAt            time.Time
	lastUploadBytes   int64
	lastDownloadBytes int64
	uploadRate        uint64
	downloadRate      uint64
}

// Watch sends the Session's sync and network activity snapshot, then each
// snapshot that differs from the last one sent, until ctx ends or send fails.
func Watch(
	ctx context.Context,
	sess session.Session,
	send func(*s4wave_session.WatchSyncStatusResponse) error,
) error {
	rate := &syncStatusRateState{}
	var prev *s4wave_session.WatchSyncStatusResponse
	for {
		resp, waitChs := buildSyncStatusSnapshot(sess, rate, time.Now())
		if prev == nil || !resp.EqualVT(prev) {
			if err := send(resp); err != nil {
				return err
			}
			prev = resp.CloneVT()
		}
		if err := waitSyncStatus(ctx, waitChs); err != nil {
			return err
		}
	}
}

// Snapshot returns the Session's sync snapshot and the channels that close
// when it changes. The snapshot carries no transfer rates.
func Snapshot(
	sess session.Session,
	now time.Time,
) (*s4wave_session.WatchSyncStatusResponse, []<-chan struct{}) {
	return buildSyncStatusSnapshot(sess, nil, now)
}

func buildSyncStatusSnapshot(
	sess session.Session,
	rate *syncStatusRateState,
	now time.Time,
) (*s4wave_session.WatchSyncStatusResponse, []<-chan struct{}) {
	switch acc := sess.GetProviderAccount().(type) {
	case *provider_spacewave.ProviderAccount:
		return buildSpacewaveSyncStatusSnapshot(sess, acc, rate, now)
	case *provider_local.ProviderAccount:
		return buildLocalSyncStatusSnapshot(sess, acc, rate, now)
	default:
		return &s4wave_session.WatchSyncStatusResponse{}, nil
	}
}

func buildSpacewaveSyncStatusSnapshot(
	sess session.Session,
	acc *provider_spacewave.ProviderAccount,
	rate *syncStatusRateState,
	now time.Time,
) (*s4wave_session.WatchSyncStatusResponse, []<-chan struct{}) {
	// Read the account status under the account broadcast.
	status, accountCh := acc.SnapshotAccountStatus()

	// Read sync telemetry and transport composition for this session.
	telemetry, telemetryCh := acc.GetSyncTelemetrySnapshotWithWait()
	sessionID := ""
	if ref := sess.GetSessionRef(); ref != nil && ref.GetProviderResourceRef() != nil {
		sessionID = ref.GetProviderResourceRef().GetId()
	}
	composition, compositionCh := acc.GetTransportCompositionSnapshotWithWait(sessionID)
	return &s4wave_session.WatchSyncStatusResponse{
			Cloud:  syncStatusFromSpacewaveTelemetry(telemetry, rate, now),
			Access: syncStatusAccountAccess(status),
			Peer:   syncStatusFromSpacewaveComposition(composition),
		},
		[]<-chan struct{}{accountCh, telemetryCh, compositionCh}
}

// syncStatusFromSpacewaveComposition projects only the selected Session's direct path.
func syncStatusFromSpacewaveComposition(composition provider_spacewave.TransportCompositionSnapshot) *s4wave_session.SyncPeerProgress {
	return &s4wave_session.SyncPeerProgress{
		State:           syncStatusSpacewaveP2PState(composition.P2PState),
		LastError:       composition.LastError,
		ActivePeerCount: composition.ActivePeerCount,
	}
}

// syncStatusHeld returns the held state of store, or nil while the store holds
// nothing back. The function reads the count and time the store's flush derived
// together, so the function never shows a held store as publishing.
func syncStatusHeld(store provider_spacewave.SyncTelemetryBlockStoreSnapshot) *s4wave_session.SyncHeldState {
	if store.CorruptDirtyCount <= 0 {
		return nil
	}
	return &s4wave_session.SyncHeldState{
		DamagedBlockCount: uint32Count(store.CorruptDirtyCount),
		HeldAt:            timestamppb.New(store.HeldSince),
		Partial:           store.HeldPartial,
	}
}

func buildLocalSyncStatusSnapshot(
	sess session.Session,
	acc *provider_local.ProviderAccount,
	rate *syncStatusRateState,
	now time.Time,
) (*s4wave_session.WatchSyncStatusResponse, []<-chan struct{}) {
	// Read the pairing snapshot when the Session has a pairing engine.
	var pairingCh <-chan struct{}
	var pairingSnapshot pairing.Snapshot
	if supported, ok := sess.(pairing.Session); ok {
		if engine, err := supported.GetPairingEngine(); err == nil {
			pairingSnapshot, pairingCh = engine.Snapshot()
		}
	}

	// Start the local snapshot from the transport and P2P running flags.
	transportRunning, transportCh := acc.GetTransportSnapshotWithWait()
	p2pRunning, p2pCh := acc.GetP2PSyncSnapshotWithWait()
	resp := syncStatusFromLocalState(pairingSnapshot, transportRunning, p2pRunning)

	// List the Spaces the account replicates while P2P sync is running.
	progress, copyCh := acc.GetAccountCopyProgress()
	var inventory *sobject.SharedObjectList
	if p2pRunning {
		inventory = acc.GetSharedObjectList()
	}
	for _, entry := range inventory.GetSharedObjects() {
		if !provider_local.ReplicatesSpace(entry) {
			continue
		}
		item := &s4wave_session.SyncLocalCopyStatus{
			SharedObjectId: entry.GetRef().GetProviderResourceRef().GetId(),
			DisplayName:    "Space",
		}
		meta := &space.SpaceSoMeta{}
		if err := meta.UnmarshalVT(entry.GetMeta().GetBodyMeta()); err == nil && meta.GetName() != "" {
			item.DisplayName = meta.GetName()
		}
		for _, current := range progress {
			if current.GetObjectId() == item.SharedObjectId {
				item.Blocks, item.Bytes = current.GetBlocks(), current.GetBytes()
				item.Complete, item.Error = current.GetComplete(), current.GetError()
				break
			}
		}
		resp.LocalCopies = append(resp.LocalCopies, item)
		if !item.Complete {
			resp.Transfer.PendingDownloadCount++
		}
	}
	slices.SortFunc(resp.LocalCopies, func(a, b *s4wave_session.SyncLocalCopyStatus) int {
		if a.DisplayName != b.DisplayName {
			return strings.Compare(a.DisplayName, b.DisplayName)
		}
		return strings.Compare(a.SharedObjectId, b.SharedObjectId)
	})

	// Attach peer transfer traffic to the snapshot.
	traffic, waits := acc.GetAccountTransferSnapshot()
	for _, peer := range traffic.Peers {
		if peer.Connected {
			resp.ActivePeerCount++
		}
		resp.Peers = append(resp.Peers, &s4wave_session.SyncPeerTransferStatus{
			PeerId: peer.PeerID, UploadedBytes: peer.UploadedBytes, DownloadedBytes: peer.DownloadedBytes, Connected: peer.Connected,
		})
	}
	resp.PeerUploadBytes, resp.PeerDownloadBytes = traffic.UploadedBytes, traffic.DownloadedBytes
	if !traffic.LastActivity.IsZero() {
		resp.Transfer.LastActivityAt = timestamppb.New(traffic.LastActivity)
	}

	// Mark the snapshot active, apply rates, and set direction and P2P state.
	if resp.Transfer.PendingDownloadCount > 0 || now.Sub(traffic.LastActivity) < 2*syncStatusRateWindow {
		resp.Transfer.State = s4wave_session.SyncStatusState_SyncStatusState_ACTIVE
	}
	if rate != nil {
		rate.apply(resp.Transfer, syncStatusCounters{
			uploadBytes:   signedTransferCounter(traffic.UploadedBytes),
			downloadBytes: signedTransferCounter(traffic.DownloadedBytes),
		}, now)
	}
	if resp.Transfer.UploadBytesPerSecond > 0 {
		resp.Transfer.Direction = s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD
	}
	if resp.Transfer.DownloadBytesPerSecond > 0 || resp.Transfer.PendingDownloadCount > 0 {
		if resp.Transfer.Direction == s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD {
			resp.Transfer.Direction = s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD_DOWNLOAD
		} else {
			resp.Transfer.Direction = s4wave_session.SyncActivityDirection_SyncActivityDirection_DOWNLOAD
		}
	}
	if resp.ActivePeerCount == 0 {
		resp.State = s4wave_session.SyncP2PState_SyncP2PState_NO_PEERS
	}
	return &s4wave_session.WatchSyncStatusResponse{
		Access: s4wave_session.SyncAccountAccessState_SyncAccountAccessState_LOCAL,
		Peer:   resp,
	}, append(waits, pairingCh, transportCh, p2pCh, copyCh)
}

// syncUploadHold returns the status form of the account limit holding uploads.
func syncUploadHold(hold provider_spacewave.SyncTelemetryUploadHold) s4wave_session.SyncUploadHold {
	switch hold {
	case provider_spacewave.SyncTelemetryUploadHoldStorageFull:
		return s4wave_session.SyncUploadHold_SyncUploadHold_STORAGE_FULL
	case provider_spacewave.SyncTelemetryUploadHoldSpendingLimit:
		return s4wave_session.SyncUploadHold_SyncUploadHold_SPENDING_LIMIT
	}
	return s4wave_session.SyncUploadHold_SyncUploadHold_NONE
}

func syncStatusFromSpacewaveTelemetry(
	telemetry provider_spacewave.SyncTelemetrySnapshot,
	rate *syncStatusRateState,
	now time.Time,
) *s4wave_session.SyncCloudProgress {
	// Copy Spacewave telemetry into a synced snapshot.
	resp := &s4wave_session.SyncCloudProgress{
		Transfer: &s4wave_session.SyncTransferProgress{
			State:                        s4wave_session.SyncStatusState_SyncStatusState_SYNCED,
			Direction:                    syncStatusDirection(telemetry),
			PendingUploadBytes:           nonNegativeUint64(telemetry.PendingUploadBytes),
			PendingDownloadBytes:         0,
			PendingUploadCount:           uint32Count(telemetry.PendingUploadCount),
			PendingDownloadCount:         0,
			ActiveUploadBytes:            nonNegativeUint64(telemetry.ActiveUploadBytes),
			ActiveUploadTransferredBytes: nonNegativeUint64(telemetry.ActiveUploadTransferredBytes),
			InFlightUploadCount:          uint32Count(telemetry.InFlightPushes),
			LastError:                    telemetry.LastError,
		},
		ActiveStoreCount:                  uint32Count(telemetry.StoreCount),
		UploadError:                       telemetry.UploadError,
		UploadHold:                        syncUploadHold(telemetry.UploadHold),
		PackRangeRequestCount:             telemetry.RangeRequestCount,
		PackRangeResponseBytes:            nonNegativeUint64(telemetry.RangeResponseBytes),
		PackFullResponseFallbackCount:     telemetry.FullResponseFallbackCount,
		PackFullResponseFallbackBytes:     nonNegativeUint64(telemetry.FullResponseFallbackBytes),
		PackLastFullResponseFallbackBytes: nonNegativeUint64(telemetry.LastFullResponseFallback),
		PackManifestEntries:               uint32Count(telemetry.ManifestEntries),
		PackBlockCountTotal:               telemetry.PackBlockCountTotal,
		PackBlockCountMin:                 telemetry.PackBlockCountMin,
		PackBlockCountMax:                 telemetry.PackBlockCountMax,
		PackSizeBytesTotal:                telemetry.PackSizeBytesTotal,
		PackSizeBytesMin:                  telemetry.PackSizeBytesMin,
		PackSizeBytesMax:                  telemetry.PackSizeBytesMax,
		PackBloomFilterCount:              uint32Count(telemetry.BloomFilterCount),
		PackBloomMissingCount:             uint32Count(telemetry.BloomMissingCount),
		PackBloomInvalidCount:             uint32Count(telemetry.BloomInvalidCount),
		PackBloomMaxFalsePositiveRate:     telemetry.BloomMaxFalsePositiveRate,
		PackBloomRiskPackCount:            uint32Count(telemetry.BloomRiskPackCount),
		PackLookupCount:                   telemetry.LookupCount,
		PackCandidatePacks:                telemetry.CandidatePacks,
		PackOpenedPacks:                   telemetry.OpenedPacks,
		PackNegativePacks:                 telemetry.NegativePacks,
		PackTargetHits:                    telemetry.TargetHits,
		PackLastCandidatePacks:            uint32Count(telemetry.LastCandidatePacks),
		PackLastOpenedPacks:               uint32Count(telemetry.LastOpenedPacks),
		PackLastNegativePacks:             uint32Count(telemetry.LastNegativePacks),
		PackLastTargetHit:                 telemetry.LastTargetHit,
		PackIndexCacheHits:                telemetry.IndexCacheHits,
		PackIndexCacheMisses:              telemetry.IndexCacheMisses,
		PackIndexCacheReadErrors:          telemetry.IndexCacheReadErrors,
		PackIndexCacheWriteErrors:         telemetry.IndexCacheWriteErrors,
		PackRemoteIndexLoads:              telemetry.RemoteIndexLoads,
		PackRemoteIndexBytes:              nonNegativeUint64(telemetry.RemoteIndexBytes),
		PackLastRemoteIndexBytes:          nonNegativeUint64(telemetry.LastRemoteIndexBytes),
		PackIndexTailFetchCount:           telemetry.IndexTailFetchCount,
		PackIndexTailFetchBytes:           nonNegativeUint64(telemetry.IndexTailFetchBytes),
		PackIndexTailResponseBytes:        nonNegativeUint64(telemetry.IndexTailResponseBytes),
	}

	// Copy block-store hit counters onto the snapshot.
	resp.BlockStores = make([]*s4wave_session.SyncBlockStoreStatus, 0, len(telemetry.BlockStores))
	for _, store := range telemetry.BlockStores {
		resp.BlockStores = append(resp.BlockStores, &s4wave_session.SyncBlockStoreStatus{
			BlockStoreId:             store.BlockStoreID,
			SharedObjectId:           store.SharedObjectID,
			DirectHitCount:           store.DirectHitCount,
			CloudHitCount:            store.CloudHitCount,
			CacheHitCount:            store.CacheHitCount,
			LastSource:               syncStatusBlockSource(store.LastSource),
			AcceptedCheckpointHeight: store.AcceptedCheckpointHeight,
			CloudRemoteSequence:      store.CloudRemoteSequence,
			Held:                     syncStatusHeld(store),
		})
	}

	// Classify only cloud activity and failures, then apply cloud rates.
	if !telemetry.LastActivityAt.IsZero() {
		resp.Transfer.LastActivityAt = timestamppb.New(telemetry.LastActivityAt)
	}
	if resp.Transfer.Direction != s4wave_session.SyncActivityDirection_SyncActivityDirection_NONE {
		resp.Transfer.State = s4wave_session.SyncStatusState_SyncStatusState_ACTIVE
	}
	if resp.Transfer.LastError != "" {
		resp.Transfer.State = s4wave_session.SyncStatusState_SyncStatusState_ERROR
	}
	if rate != nil {
		rate.apply(resp.Transfer, syncStatusCounters{
			uploadBytes:   telemetry.PushedBytes + telemetry.ActiveUploadTransferredBytes,
			downloadBytes: telemetry.FetchedBytes,
		}, now)
	}
	return resp
}

func signedTransferCounter(value uint64) int64 {
	if value > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(value) //nolint:gosec // the MaxInt64 check saturates the uint64 transport counter for rate arithmetic.
}

func uint32Count(value int) uint32 {
	if value <= 0 {
		return 0
	}
	if uint64(value) > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(value) //nolint:gosec // the positive MaxUint32 check bounds this telemetry count conversion.
}

func syncStatusFromLocalState(
	snapshot pairing.Snapshot,
	transportRunning bool,
	p2pRunning bool,
) *s4wave_session.SyncPeerProgress {
	p2pState, errMsg := syncStatusLocalP2PState(snapshot, transportRunning, p2pRunning)
	resp := &s4wave_session.SyncPeerProgress{
		Transfer: &s4wave_session.SyncTransferProgress{
			State:     s4wave_session.SyncStatusState_SyncStatusState_SYNCED,
			Direction: s4wave_session.SyncActivityDirection_SyncActivityDirection_NONE,
		},
		NetworkState: syncStatusLocalTransportState(transportRunning),
		State:        p2pState,
		LastError:    errMsg,
	}
	if errMsg != "" {
		resp.Transfer.State = s4wave_session.SyncStatusState_SyncStatusState_ERROR
	}
	return resp
}

func syncStatusDirection(
	telemetry provider_spacewave.SyncTelemetrySnapshot,
) s4wave_session.SyncActivityDirection {
	// Classify telemetry as upload, download, both, or idle.
	uploading := telemetry.PendingUploadBytes > 0 ||
		telemetry.PendingUploadCount > 0 ||
		telemetry.InFlightPushes > 0
	downloading := telemetry.PullActiveCount > 0 || telemetry.InFlightFetches > 0
	if uploading && downloading {
		return s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD_DOWNLOAD
	}
	if uploading {
		return s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD
	}
	if downloading {
		return s4wave_session.SyncActivityDirection_SyncActivityDirection_DOWNLOAD
	}
	return s4wave_session.SyncActivityDirection_SyncActivityDirection_NONE
}

// syncStatusAccountAccess preserves the account's access decision without inferring socket health.
func syncStatusAccountAccess(status provider.ProviderAccountStatus) s4wave_session.SyncAccountAccessState {
	switch status {
	case provider.ProviderAccountStatus_ProviderAccountStatus_READY:
		return s4wave_session.SyncAccountAccessState_SyncAccountAccessState_READY
	case provider.ProviderAccountStatus_ProviderAccountStatus_DORMANT:
		return s4wave_session.SyncAccountAccessState_SyncAccountAccessState_DORMANT
	case provider.ProviderAccountStatus_ProviderAccountStatus_PENDING, provider.ProviderAccountStatus_ProviderAccountStatus_NONE:
		return s4wave_session.SyncAccountAccessState_SyncAccountAccessState_PENDING
	case provider.ProviderAccountStatus_ProviderAccountStatus_FAILED:
		return s4wave_session.SyncAccountAccessState_SyncAccountAccessState_FAILED
	case provider.ProviderAccountStatus_ProviderAccountStatus_DELETED:
		return s4wave_session.SyncAccountAccessState_SyncAccountAccessState_DELETED
	case provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED:
		return s4wave_session.SyncAccountAccessState_SyncAccountAccessState_UNAUTHENTICATED
	default:
		return s4wave_session.SyncAccountAccessState_SyncAccountAccessState_UNKNOWN
	}
}

func syncStatusSpacewaveP2PState(
	state provider_spacewave.TransportCompositionP2PState,
) s4wave_session.SyncP2PState {
	switch state {
	case provider_spacewave.TransportCompositionP2PStateDisabled:
		return s4wave_session.SyncP2PState_SyncP2PState_DISABLED
	case provider_spacewave.TransportCompositionP2PStateStarting:
		return s4wave_session.SyncP2PState_SyncP2PState_STARTING
	case provider_spacewave.TransportCompositionP2PStateNoPeers:
		return s4wave_session.SyncP2PState_SyncP2PState_NO_PEERS
	case provider_spacewave.TransportCompositionP2PStateIdle:
		return s4wave_session.SyncP2PState_SyncP2PState_IDLE
	case provider_spacewave.TransportCompositionP2PStateActive:
		return s4wave_session.SyncP2PState_SyncP2PState_ACTIVE
	case provider_spacewave.TransportCompositionP2PStateFallbackNoPeer:
		return s4wave_session.SyncP2PState_SyncP2PState_FALLBACK_NO_PEER
	case provider_spacewave.TransportCompositionP2PStateError:
		return s4wave_session.SyncP2PState_SyncP2PState_ERROR
	default:
		return s4wave_session.SyncP2PState_SyncP2PState_UNKNOWN
	}
}

func syncStatusBlockSource(
	source provider_spacewave.SyncTelemetryBlockSource,
) s4wave_session.SyncBlockSource {
	switch source {
	case provider_spacewave.SyncTelemetryBlockSourceCache:
		return s4wave_session.SyncBlockSource_SyncBlockSource_CACHE
	case provider_spacewave.SyncTelemetryBlockSourceDirect:
		return s4wave_session.SyncBlockSource_SyncBlockSource_DIRECT
	case provider_spacewave.SyncTelemetryBlockSourceCloud:
		return s4wave_session.SyncBlockSource_SyncBlockSource_CLOUD
	default:
		return s4wave_session.SyncBlockSource_SyncBlockSource_UNKNOWN
	}
}

func syncStatusLocalTransportState(transportRunning bool) s4wave_session.SyncTransportState {
	if transportRunning {
		return s4wave_session.SyncTransportState_SyncTransportState_ONLINE
	}
	return s4wave_session.SyncTransportState_SyncTransportState_UNAVAILABLE
}

func syncStatusLocalP2PState(
	snapshot pairing.Snapshot,
	transportRunning bool,
	p2pRunning bool,
) (s4wave_session.SyncP2PState, string) {
	switch snapshot.Status {
	case pairing.StatusFailed,
		pairing.StatusSignalingFailed,
		pairing.StatusConnectionTimeout,
		pairing.StatusPairingRejected,
		pairing.StatusConfirmationTimeout:
		return s4wave_session.SyncP2PState_SyncP2PState_ERROR, snapshot.ErrMsg
	case pairing.StatusIdle:
	default:
		return s4wave_session.SyncP2PState_SyncP2PState_ACTIVE, ""
	}
	return syncStatusP2PState(transportRunning, p2pRunning, false), ""
}

func syncStatusP2PState(
	transportRunning bool,
	p2pRunning bool,
	hasError bool,
) s4wave_session.SyncP2PState {
	if hasError {
		return s4wave_session.SyncP2PState_SyncP2PState_ERROR
	}
	if p2pRunning {
		return s4wave_session.SyncP2PState_SyncP2PState_ACTIVE
	}
	if transportRunning {
		return s4wave_session.SyncP2PState_SyncP2PState_IDLE
	}
	return s4wave_session.SyncP2PState_SyncP2PState_NO_PEERS
}

func (s *syncStatusRateState) apply(
	resp *s4wave_session.SyncTransferProgress,
	counters syncStatusCounters,
	now time.Time,
) {
	// Reset the rate window when sync is not active.
	if resp.State != s4wave_session.SyncStatusState_SyncStatusState_ACTIVE {
		s.lastAt = now
		s.lastUploadBytes = counters.uploadBytes
		s.lastDownloadBytes = counters.downloadBytes
		s.uploadRate = 0
		s.downloadRate = 0
		return
	}

	// Start the window on the first active sample.
	if s.lastAt.IsZero() {
		s.lastAt = now
		s.lastUploadBytes = counters.uploadBytes
		s.lastDownloadBytes = counters.downloadBytes
		return
	}

	// Advance the window and copy the rates onto the response.
	elapsed := now.Sub(s.lastAt)
	if elapsed >= syncStatusRateWindow {
		s.uploadRate = bytesPerSecond(counters.uploadBytes-s.lastUploadBytes, elapsed)
		s.downloadRate = bytesPerSecond(counters.downloadBytes-s.lastDownloadBytes, elapsed)
		s.lastAt = now
		s.lastUploadBytes = counters.uploadBytes
		s.lastDownloadBytes = counters.downloadBytes
	}
	resp.UploadBytesPerSecond = s.uploadRate
	resp.DownloadBytesPerSecond = s.downloadRate
}

// waitSyncStatus waits on source changes and the existing transfer-rate sample deadline.
func waitSyncStatus(ctx context.Context, waitChs []<-chan struct{}) error {
	// Sample rates and clear stale activity even after the final transfer.
	waitCtx, cancel := context.WithTimeout(ctx, syncStatusRateWindow)
	defer cancel()
	err := broadcast.WaitAny(waitCtx, waitChs...)
	if ctx.Err() == nil && waitCtx.Err() == context.DeadlineExceeded {
		return nil
	}
	return err
}

func bytesPerSecond(delta int64, elapsed time.Duration) uint64 {
	if delta <= 0 || elapsed <= 0 {
		return 0
	}
	return uint64(float64(delta) / elapsed.Seconds())
}

func nonNegativeUint64(v int64) uint64 {
	if v <= 0 {
		return 0
	}
	return uint64(v)
}
