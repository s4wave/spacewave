package resource_session

import (
	"context"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
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

func (r *SessionResource) buildSyncStatusSnapshot(
	rate *syncStatusRateState,
	now time.Time,
) (*s4wave_session.WatchSyncStatusResponse, []<-chan struct{}) {
	switch acc := r.session.GetProviderAccount().(type) {
	case *provider_spacewave.ProviderAccount:
		return r.buildSpacewaveSyncStatusSnapshot(acc, rate, now)
	case *provider_local.ProviderAccount:
		return r.buildLocalSyncStatusSnapshot(acc)
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
	transportRunning, transportCh := acc.GetTransportSnapshotWithWait()
	return syncStatusFromSpacewaveTelemetry(telemetry, status, transportRunning, rate, now),
		[]<-chan struct{}{accountCh, telemetryCh, transportCh}
}

func (r *SessionResource) buildLocalSyncStatusSnapshot(
	acc *provider_local.ProviderAccount,
) (*s4wave_session.WatchSyncStatusResponse, []<-chan struct{}) {
	var pairingCh <-chan struct{}
	var pairing provider_local.PairingSnapshot
	acc.GetPairingBroadcast().HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		pairingCh = getWaitCh()
		pairing = acc.GetPairingSnapshot()
	})
	transportRunning, transportCh := acc.GetTransportSnapshotWithWait()
	return syncStatusFromLocalState(pairing, transportRunning, acc.IsP2PSyncRunning()),
		[]<-chan struct{}{pairingCh, transportCh}
}

func syncStatusFromSpacewaveTelemetry(
	telemetry provider_spacewave.SyncTelemetrySnapshot,
	status provider.ProviderAccountStatus,
	transportRunning bool,
	rate *syncStatusRateState,
	now time.Time,
) *s4wave_session.WatchSyncStatusResponse {
	resp := &s4wave_session.WatchSyncStatusResponse{
		State:                             s4wave_session.SyncStatusState_SyncStatusState_SYNCED,
		Direction:                         syncStatusDirection(telemetry),
		TransportState:                    syncStatusSpacewaveTransportState(status, transportRunning),
		P2PState:                          syncStatusP2PState(transportRunning, false, false),
		PendingUploadBytes:                nonNegativeUint64(telemetry.PendingUploadBytes),
		PendingDownloadBytes:              0,
		PendingUploadCount:                uint32(max(telemetry.PendingUploadCount, 0)),
		PendingDownloadCount:              0,
		ActiveUploadBytes:                 nonNegativeUint64(telemetry.ActiveUploadBytes),
		ActiveUploadTransferredBytes:      nonNegativeUint64(telemetry.ActiveUploadTransferredBytes),
		InFlightUploadCount:               uint32(max(telemetry.InFlightPushes, 0)),
		ActiveStoreCount:                  uint32(max(telemetry.StoreCount, 0)),
		LastError:                         telemetry.LastError,
		PackRangeRequestCount:             telemetry.RangeRequestCount,
		PackRangeResponseBytes:            nonNegativeUint64(telemetry.RangeResponseBytes),
		PackFullResponseFallbackCount:     telemetry.FullResponseFallbackCount,
		PackFullResponseFallbackBytes:     nonNegativeUint64(telemetry.FullResponseFallbackBytes),
		PackLastFullResponseFallbackBytes: nonNegativeUint64(telemetry.LastFullResponseFallback),
		PackManifestEntries:               uint32(max(telemetry.ManifestEntries, 0)),
		PackBlockCountTotal:               telemetry.PackBlockCountTotal,
		PackBlockCountMin:                 telemetry.PackBlockCountMin,
		PackBlockCountMax:                 telemetry.PackBlockCountMax,
		PackSizeBytesTotal:                telemetry.PackSizeBytesTotal,
		PackSizeBytesMin:                  telemetry.PackSizeBytesMin,
		PackSizeBytesMax:                  telemetry.PackSizeBytesMax,
		PackBloomFilterCount:              uint32(max(telemetry.BloomFilterCount, 0)),
		PackBloomMissingCount:             uint32(max(telemetry.BloomMissingCount, 0)),
		PackBloomInvalidCount:             uint32(max(telemetry.BloomInvalidCount, 0)),
		PackBloomParameterShapeCount:      uint32(max(telemetry.BloomParameterShapeCount, 0)),
		PackBloomMaxFalsePositiveRate:     telemetry.BloomMaxFalsePositiveRate,
		PackBloomRiskPackCount:            uint32(max(telemetry.BloomRiskPackCount, 0)),
		PackLookupCount:                   telemetry.LookupCount,
		PackCandidatePacks:                telemetry.CandidatePacks,
		PackOpenedPacks:                   telemetry.OpenedPacks,
		PackNegativePacks:                 telemetry.NegativePacks,
		PackTargetHits:                    telemetry.TargetHits,
		PackLastCandidatePacks:            uint32(max(telemetry.LastCandidatePacks, 0)),
		PackLastOpenedPacks:               uint32(max(telemetry.LastOpenedPacks, 0)),
		PackLastNegativePacks:             uint32(max(telemetry.LastNegativePacks, 0)),
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
	if !telemetry.LastActivityAt.IsZero() {
		resp.LastActivityAt = timestamppb.New(telemetry.LastActivityAt)
	}
	if resp.Direction != s4wave_session.SyncActivityDirection_SyncActivityDirection_NONE {
		resp.State = s4wave_session.SyncStatusState_SyncStatusState_ACTIVE
	}
	if telemetry.LastError != "" {
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

func syncStatusFromLocalState(
	pairing provider_local.PairingSnapshot,
	transportRunning bool,
	p2pRunning bool,
) *s4wave_session.WatchSyncStatusResponse {
	p2pState, errMsg := syncStatusLocalP2PState(pairing, transportRunning, p2pRunning)
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
	transportRunning bool,
) s4wave_session.SyncTransportState {
	if transportRunning {
		return s4wave_session.SyncTransportState_SyncTransportState_ONLINE
	}
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

func syncStatusLocalTransportState(transportRunning bool) s4wave_session.SyncTransportState {
	if transportRunning {
		return s4wave_session.SyncTransportState_SyncTransportState_ONLINE
	}
	return s4wave_session.SyncTransportState_SyncTransportState_UNAVAILABLE
}

func syncStatusLocalP2PState(
	pairing provider_local.PairingSnapshot,
	transportRunning bool,
	p2pRunning bool,
) (s4wave_session.SyncP2PState, string) {
	switch pairing.Status {
	case provider_local.PairingStatusFailed,
		provider_local.PairingStatusSignalingFailed,
		provider_local.PairingStatusConnectionTimeout,
		provider_local.PairingStatusPairingRejected,
		provider_local.PairingStatusConfirmationTimeout:
		return s4wave_session.SyncP2PState_SyncP2PState_ERROR, pairing.ErrMsg
	case provider_local.PairingStatusIdle:
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
	chans := make([]<-chan struct{}, 0, len(waitChs))
	for _, ch := range waitChs {
		if ch != nil {
			chans = append(chans, ch)
		}
	}
	switch len(chans) {
	case 0:
		<-ctx.Done()
	case 1:
		select {
		case <-ctx.Done():
		case <-chans[0]:
		}
	case 2:
		select {
		case <-ctx.Done():
		case <-chans[0]:
		case <-chans[1]:
		}
	default:
		select {
		case <-ctx.Done():
		case <-chans[0]:
		case <-chans[1]:
		case <-chans[2]:
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
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
