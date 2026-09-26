package resource_session

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

// WatchSyncStatus streams the session sync and network activity snapshot.
func (r *SessionResource) WatchSyncStatus(
	req *s4wave_session.WatchSyncStatusRequest,
	strm s4wave_session.SRPCSessionResourceService_WatchSyncStatusStream,
) error {
	ctx := strm.Context()
	rate := &syncStatusRateState{}
	var prev *s4wave_session.WatchSyncStatusResponse
	for {
		resp, waitChs := r.buildSyncStatusSnapshot(rate, time.Now())
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp.CloneVT()
		}
		if err := waitSyncStatus(ctx, waitChs); err != nil {
			return err
		}
	}
}

// BuildSyncStatusSnapshot returns the session sync snapshot and change channels.
func BuildSyncStatusSnapshot(
	sess session.Session,
	now time.Time,
) (*s4wave_session.WatchSyncStatusResponse, []<-chan struct{}) {
	return (&SessionResource{session: sess}).buildSyncStatusSnapshot(nil, now)
}

func (r *SessionResource) buildSyncStatusSnapshot(
	rate *syncStatusRateState,
	now time.Time,
) (*s4wave_session.WatchSyncStatusResponse, []<-chan struct{}) {
	switch acc := r.session.GetProviderAccount().(type) {
	case *provider_spacewave.ProviderAccount:
		return r.buildSpacewaveSyncStatusSnapshot(acc, rate, now)
	case *provider_local.ProviderAccount:
		return r.buildLocalSyncStatusSnapshot(acc, rate, now)
	default:
		return &s4wave_session.WatchSyncStatusResponse{
			State:          s4wave_session.SyncStatusState_SyncStatusState_SYNCED,
			Direction:      s4wave_session.SyncActivityDirection_SyncActivityDirection_NONE,
			TransportState: s4wave_session.SyncTransportState_SyncTransportState_UNKNOWN,
			P2PState:       s4wave_session.SyncP2PState_SyncP2PState_UNKNOWN,
		}, nil
	}
}

func (r *SessionResource) buildSpacewaveSyncStatusSnapshot(
	acc *provider_spacewave.ProviderAccount,
	rate *syncStatusRateState,
	now time.Time,
) (*s4wave_session.WatchSyncStatusResponse, []<-chan struct{}) {
	var accountCh <-chan struct{}
	var status provider.ProviderAccountStatus
	acc.GetAccountBroadcast().HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		accountCh = getWaitCh()
		status = acc.GetAccountStatus()
	})

	telemetry, telemetryCh := acc.GetSyncTelemetrySnapshotWithWait()
	sessionID := ""
	if ref := r.session.GetSessionRef(); ref != nil && ref.GetProviderResourceRef() != nil {
		sessionID = ref.GetProviderResourceRef().GetId()
	}
	composition, compositionCh := acc.GetTransportCompositionSnapshotWithWait(sessionID)
	return syncStatusFromSpacewaveTelemetry(telemetry, status, composition, rate, now),
		[]<-chan struct{}{accountCh, telemetryCh, compositionCh}
}

func (r *SessionResource) buildLocalSyncStatusSnapshot(
	acc *provider_local.ProviderAccount,
	rate *syncStatusRateState,
	now time.Time,
) (*s4wave_session.WatchSyncStatusResponse, []<-chan struct{}) {
	var pairingCh <-chan struct{}
	var pairingSnapshot pairing.Snapshot
	if engine, err := r.getPairingEngine(); err == nil {
		pairingSnapshot, pairingCh = engine.Snapshot()
	}
	transportRunning, transportCh := acc.GetTransportSnapshotWithWait()
	p2pRunning, p2pCh := acc.GetP2PSyncSnapshotWithWait()
	resp := syncStatusFromLocalState(pairingSnapshot, transportRunning, p2pRunning)
	resp.LocalAccount = true
	progress, copyCh := acc.GetAccountCopyProgress()
	var inventory *sobject.SharedObjectList
	if p2pRunning {
		inventory = acc.GetSOListCtr().GetValue()
	}
	for _, entry := range inventory.GetSharedObjects() {
		if entry.GetMeta().GetBodyType() != "space" {
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
			resp.PendingDownloadCount++
		}
	}
	slices.SortFunc(resp.LocalCopies, func(a, b *s4wave_session.SyncLocalCopyStatus) int {
		if a.DisplayName != b.DisplayName {
			return strings.Compare(a.DisplayName, b.DisplayName)
		}
		return strings.Compare(a.SharedObjectId, b.SharedObjectId)
	})
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
		resp.LastActivityAt = timestamppb.New(traffic.LastActivity)
	}
	if resp.PendingDownloadCount > 0 || now.Sub(traffic.LastActivity) < 2*syncStatusRateWindow {
		resp.State = s4wave_session.SyncStatusState_SyncStatusState_ACTIVE
	}
	if rate != nil {
		rate.apply(resp, syncStatusCounters{uploadBytes: signedTransferCounter(traffic.UploadedBytes), downloadBytes: signedTransferCounter(traffic.DownloadedBytes)}, now)
	}
	if resp.UploadBytesPerSecond > 0 {
		resp.Direction = s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD
	}
	if resp.DownloadBytesPerSecond > 0 || resp.PendingDownloadCount > 0 {
		if resp.Direction == s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD {
			resp.Direction = s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD_DOWNLOAD
		} else {
			resp.Direction = s4wave_session.SyncActivityDirection_SyncActivityDirection_DOWNLOAD
		}
	}
	if resp.ActivePeerCount == 0 {
		resp.P2PState = s4wave_session.SyncP2PState_SyncP2PState_NO_PEERS
	}
	return resp, append(waits, pairingCh, transportCh, p2pCh, copyCh)
}

func syncStatusFromSpacewaveTelemetry(
	telemetry provider_spacewave.SyncTelemetrySnapshot,
	status provider.ProviderAccountStatus,
	composition provider_spacewave.TransportCompositionSnapshot,
	rate *syncStatusRateState,
	now time.Time,
) *s4wave_session.WatchSyncStatusResponse {
	resp := &s4wave_session.WatchSyncStatusResponse{
		State:                             s4wave_session.SyncStatusState_SyncStatusState_SYNCED,
		Direction:                         syncStatusDirection(telemetry),
		TransportState:                    syncStatusSpacewaveTransportState(status),
		P2PState:                          syncStatusSpacewaveP2PState(composition.P2PState),
		PendingUploadBytes:                nonNegativeUint64(telemetry.PendingUploadBytes),
		PendingDownloadBytes:              0,
		PendingUploadCount:                uint32Count(telemetry.PendingUploadCount),
		PendingDownloadCount:              0,
		ActiveUploadBytes:                 nonNegativeUint64(telemetry.ActiveUploadBytes),
		ActiveUploadTransferredBytes:      nonNegativeUint64(telemetry.ActiveUploadTransferredBytes),
		InFlightUploadCount:               uint32Count(telemetry.InFlightPushes),
		ActiveStoreCount:                  uint32Count(telemetry.StoreCount),
		ActivePeerCount:                   composition.ActivePeerCount,
		DirectP2PDisabled:                 !composition.DirectP2PEnabled,
		LastError:                         telemetry.LastError,
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
	resp.BlockStores = make([]*s4wave_session.SyncBlockStoreStatus, 0, len(telemetry.BlockStores))
	for _, store := range telemetry.BlockStores {
		resp.BlockStores = append(resp.BlockStores, &s4wave_session.SyncBlockStoreStatus{
			BlockStoreId:              store.BlockStoreID,
			SharedObjectId:            store.SharedObjectID,
			DirectHitCount:            store.DirectHitCount,
			CloudHitCount:             store.CloudHitCount,
			CacheHitCount:             store.CacheHitCount,
			LastSource:                syncStatusBlockSource(store.LastSource),
			AcceptedRootInnerSequence: store.AcceptedRootInnerSequence,
			CloudRemoteSequence:       store.CloudRemoteSequence,
		})
	}
	if composition.LastError != "" {
		resp.LastError = composition.LastError
	}
	if !telemetry.LastActivityAt.IsZero() {
		resp.LastActivityAt = timestamppb.New(telemetry.LastActivityAt)
	}
	if resp.Direction != s4wave_session.SyncActivityDirection_SyncActivityDirection_NONE {
		resp.State = s4wave_session.SyncStatusState_SyncStatusState_ACTIVE
	}
	if resp.LastError != "" {
		resp.State = s4wave_session.SyncStatusState_SyncStatusState_ERROR
	}
	if rate != nil {
		rate.apply(resp, syncStatusCounters{
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
) *s4wave_session.WatchSyncStatusResponse {
	p2pState, errMsg := syncStatusLocalP2PState(snapshot, transportRunning, p2pRunning)
	resp := &s4wave_session.WatchSyncStatusResponse{
		State:          s4wave_session.SyncStatusState_SyncStatusState_SYNCED,
		Direction:      s4wave_session.SyncActivityDirection_SyncActivityDirection_NONE,
		TransportState: syncStatusLocalTransportState(transportRunning),
		P2PState:       p2pState,
		LastError:      errMsg,
	}
	if errMsg != "" {
		resp.State = s4wave_session.SyncStatusState_SyncStatusState_ERROR
	}
	return resp
}

func syncStatusDirection(
	telemetry provider_spacewave.SyncTelemetrySnapshot,
) s4wave_session.SyncActivityDirection {
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

func syncStatusSpacewaveTransportState(
	status provider.ProviderAccountStatus,
) s4wave_session.SyncTransportState {
	switch status {
	case provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		provider.ProviderAccountStatus_ProviderAccountStatus_DORMANT:
		return s4wave_session.SyncTransportState_SyncTransportState_ONLINE
	case provider.ProviderAccountStatus_ProviderAccountStatus_PENDING,
		provider.ProviderAccountStatus_ProviderAccountStatus_NONE:
		return s4wave_session.SyncTransportState_SyncTransportState_CONNECTING
	case provider.ProviderAccountStatus_ProviderAccountStatus_FAILED,
		provider.ProviderAccountStatus_ProviderAccountStatus_DELETED,
		provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED:
		return s4wave_session.SyncTransportState_SyncTransportState_ERROR
	default:
		return s4wave_session.SyncTransportState_SyncTransportState_UNKNOWN
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
	resp *s4wave_session.WatchSyncStatusResponse,
	counters syncStatusCounters,
	now time.Time,
) {
	if resp.State != s4wave_session.SyncStatusState_SyncStatusState_ACTIVE {
		s.lastAt = now
		s.lastUploadBytes = counters.uploadBytes
		s.lastDownloadBytes = counters.downloadBytes
		s.uploadRate = 0
		s.downloadRate = 0
		return
	}
	if s.lastAt.IsZero() {
		s.lastAt = now
		s.lastUploadBytes = counters.uploadBytes
		s.lastDownloadBytes = counters.downloadBytes
		return
	}
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
