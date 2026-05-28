package resource_session

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

type testWatchSyncStatusStream struct {
	srpc.Stream
	ctx  context.Context
	msgs chan *s4wave_session.WatchSyncStatusResponse
}

func newTestWatchSyncStatusStream(ctx context.Context) *testWatchSyncStatusStream {
	return &testWatchSyncStatusStream{
		ctx:  ctx,
		msgs: make(chan *s4wave_session.WatchSyncStatusResponse, 4),
	}
}

func (m *testWatchSyncStatusStream) Context() context.Context {
	return m.ctx
}

func (m *testWatchSyncStatusStream) Send(resp *s4wave_session.WatchSyncStatusResponse) error {
	select {
	case m.msgs <- resp:
		return nil
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
}

func (m *testWatchSyncStatusStream) SendAndClose(resp *s4wave_session.WatchSyncStatusResponse) error {
	return m.Send(resp)
}

func (m *testWatchSyncStatusStream) MsgRecv(_ srpc.Message) error {
	return nil
}

func (m *testWatchSyncStatusStream) MsgSend(_ srpc.Message) error {
	return nil
}

func (m *testWatchSyncStatusStream) CloseSend() error {
	return nil
}

func (m *testWatchSyncStatusStream) Close() error {
	return nil
}

type testSyncStatusSession struct {
	acc provider.ProviderAccount
}

func (s *testSyncStatusSession) GetBus() bus.Bus {
	return nil
}

func (s *testSyncStatusSession) GetSessionRef() *session.SessionRef {
	return nil
}

func (s *testSyncStatusSession) GetPeerId() peer.ID {
	return ""
}

func (s *testSyncStatusSession) GetPrivKey() crypto.PrivKey {
	return nil
}

func (s *testSyncStatusSession) GetProviderAccount() provider.ProviderAccount {
	return s.acc
}

func (s *testSyncStatusSession) AccessStateAtomStore(
	context.Context,
	string,
) (resource_state.StateAtomStore, error) {
	return nil, errors.New("not implemented")
}

func (s *testSyncStatusSession) SnapshotStateAtomStoreIDs(context.Context) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (s *testSyncStatusSession) WatchStateAtomStoreIDs(
	context.Context,
	func(storeIDs []string) error,
) error {
	return errors.New("not implemented")
}

func (s *testSyncStatusSession) GetLockState(
	context.Context,
) (session.SessionLockMode, bool, error) {
	return session.SessionLockMode_SESSION_LOCK_MODE_AUTO_UNLOCK, false, errors.New("not implemented")
}

func (s *testSyncStatusSession) WatchLockState(
	context.Context,
	func(mode session.SessionLockMode, locked bool),
) error {
	return errors.New("not implemented")
}

func (s *testSyncStatusSession) UnlockSession(context.Context, []byte) error {
	return errors.New("not implemented")
}

func (s *testSyncStatusSession) SetLockMode(
	context.Context,
	session.SessionLockMode,
	[]byte,
) error {
	return errors.New("not implemented")
}

func (s *testSyncStatusSession) LockSession(context.Context) error {
	return errors.New("not implemented")
}

func TestWatchSyncStatusInitialSnapshots(t *testing.T) {
	t.Parallel()

	spacewaveAcc := &provider_spacewave.ProviderAccount{}
	spacewaveAcc.SetAccountStatus(provider.ProviderAccountStatus_ProviderAccountStatus_READY)
	tests := []struct {
		name      string
		acc       provider.ProviderAccount
		transport s4wave_session.SyncTransportState
	}{
		{
			name:      "local",
			acc:       &provider_local.ProviderAccount{},
			transport: s4wave_session.SyncTransportState_SyncTransportState_UNAVAILABLE,
		},
		{
			name:      "spacewave",
			acc:       spacewaveAcc,
			transport: s4wave_session.SyncTransportState_SyncTransportState_ONLINE,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			strm := newTestWatchSyncStatusStream(ctx)
			res := &SessionResource{
				session: &testSyncStatusSession{acc: test.acc},
			}
			errCh := make(chan error, 1)
			go func() {
				errCh <- res.WatchSyncStatus(&s4wave_session.WatchSyncStatusRequest{}, strm)
			}()

			resp := recvSyncStatusResponse(t, strm.msgs)
			if resp.GetState() != s4wave_session.SyncStatusState_SyncStatusState_SYNCED {
				t.Fatalf("state = %v, want synced", resp.GetState())
			}
			if resp.GetDirection() != s4wave_session.SyncActivityDirection_SyncActivityDirection_NONE {
				t.Fatalf("direction = %v, want none", resp.GetDirection())
			}
			if resp.GetTransportState() != test.transport {
				t.Fatalf("transport = %v, want %v", resp.GetTransportState(), test.transport)
			}
			if resp.GetP2PState() != s4wave_session.SyncP2PState_SyncP2PState_NO_PEERS {
				t.Fatalf("p2p = %v, want no peers", resp.GetP2PState())
			}
			if resp.GetUploadBytesPerSecond() != 0 || resp.GetDownloadBytesPerSecond() != 0 {
				t.Fatalf(
					"throughput = %d/%d, want idle",
					resp.GetUploadBytesPerSecond(),
					resp.GetDownloadBytesPerSecond(),
				)
			}

			cancel()
			if err := <-errCh; err != context.Canceled {
				t.Fatalf("WatchSyncStatus() = %v, want context canceled", err)
			}
		})
	}
}

func TestSyncStatusSpacewaveAggregation(t *testing.T) {
	t.Parallel()

	status := provider.ProviderAccountStatus_ProviderAccountStatus_READY
	tests := []struct {
		name      string
		telemetry provider_spacewave.SyncTelemetrySnapshot
		state     s4wave_session.SyncStatusState
		direction s4wave_session.SyncActivityDirection
	}{
		{
			name:      "synced",
			state:     s4wave_session.SyncStatusState_SyncStatusState_SYNCED,
			direction: s4wave_session.SyncActivityDirection_SyncActivityDirection_NONE,
		},
		{
			name: "uploading",
			telemetry: provider_spacewave.SyncTelemetrySnapshot{
				PendingUploadBytes: 128,
				PendingUploadCount: 1,
				ActiveUploadBytes:  64,
				InFlightPushes:     1,
			},
			state:     s4wave_session.SyncStatusState_SyncStatusState_ACTIVE,
			direction: s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD,
		},
		{
			name: "downloading",
			telemetry: provider_spacewave.SyncTelemetrySnapshot{
				InFlightFetches: 1,
			},
			state:     s4wave_session.SyncStatusState_SyncStatusState_ACTIVE,
			direction: s4wave_session.SyncActivityDirection_SyncActivityDirection_DOWNLOAD,
		},
		{
			name: "mixed",
			telemetry: provider_spacewave.SyncTelemetrySnapshot{
				PendingUploadBytes: 128,
				InFlightFetches:    1,
			},
			state:     s4wave_session.SyncStatusState_SyncStatusState_ACTIVE,
			direction: s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD_DOWNLOAD,
		},
		{
			name: "error",
			telemetry: provider_spacewave.SyncTelemetrySnapshot{
				LastError: "sync failed",
			},
			state:     s4wave_session.SyncStatusState_SyncStatusState_ERROR,
			direction: s4wave_session.SyncActivityDirection_SyncActivityDirection_NONE,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resp := syncStatusFromSpacewaveTelemetry(
				test.telemetry,
				status,
				false,
				&syncStatusRateState{},
				time.Unix(20, 0),
			)
			if resp.GetState() != test.state {
				t.Fatalf("state = %v, want %v", resp.GetState(), test.state)
			}
			if resp.GetDirection() != test.direction {
				t.Fatalf("direction = %v, want %v", resp.GetDirection(), test.direction)
			}
			if resp.GetTransportState() != s4wave_session.SyncTransportState_SyncTransportState_ONLINE {
				t.Fatalf("transport = %v, want online", resp.GetTransportState())
			}
			if test.name == "uploading" {
				if resp.GetActiveUploadBytes() != 64 || resp.GetInFlightUploadCount() != 1 {
					t.Fatalf("active upload fields = %d/%d, want 64/1",
						resp.GetActiveUploadBytes(),
						resp.GetInFlightUploadCount(),
					)
				}
			}
		})
	}
}

func TestSyncStatusLocalNoPeerAggregation(t *testing.T) {
	t.Parallel()

	resp := syncStatusFromLocalState(
		provider_local.PairingSnapshot{Status: provider_local.PairingStatusIdle},
		false,
		false,
	)
	if resp.GetState() != s4wave_session.SyncStatusState_SyncStatusState_SYNCED {
		t.Fatalf("state = %v, want synced", resp.GetState())
	}
	if resp.GetTransportState() != s4wave_session.SyncTransportState_SyncTransportState_UNAVAILABLE {
		t.Fatalf("transport = %v, want unavailable", resp.GetTransportState())
	}
	if resp.GetP2PState() != s4wave_session.SyncP2PState_SyncP2PState_NO_PEERS {
		t.Fatalf("p2p = %v, want no peers", resp.GetP2PState())
	}
}

func TestSyncStatusLocalP2PLifecycleAggregation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pairing    provider_local.PairingSnapshot
		transport  bool
		p2pRunning bool
		state      s4wave_session.SyncStatusState
		p2p        s4wave_session.SyncP2PState
		errMsg     string
	}{
		{
			name:      "transport idle",
			transport: true,
			state:     s4wave_session.SyncStatusState_SyncStatusState_SYNCED,
			p2p:       s4wave_session.SyncP2PState_SyncP2PState_IDLE,
		},
		{
			name: "pairing active",
			pairing: provider_local.PairingSnapshot{
				Status: provider_local.PairingStatusWaitingForPeer,
			},
			transport: true,
			state:     s4wave_session.SyncStatusState_SyncStatusState_SYNCED,
			p2p:       s4wave_session.SyncP2PState_SyncP2PState_ACTIVE,
		},
		{
			name:       "p2p sync running",
			transport:  true,
			p2pRunning: true,
			state:      s4wave_session.SyncStatusState_SyncStatusState_SYNCED,
			p2p:        s4wave_session.SyncP2PState_SyncP2PState_ACTIVE,
		},
		{
			name: "pairing error",
			pairing: provider_local.PairingSnapshot{
				Status: provider_local.PairingStatusFailed,
				ErrMsg: "pairing failed",
			},
			transport: true,
			state:     s4wave_session.SyncStatusState_SyncStatusState_ERROR,
			p2p:       s4wave_session.SyncP2PState_SyncP2PState_ERROR,
			errMsg:    "pairing failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resp := syncStatusFromLocalState(test.pairing, test.transport, test.p2pRunning)
			if resp.GetState() != test.state {
				t.Fatalf("state = %v, want %v", resp.GetState(), test.state)
			}
			if resp.GetP2PState() != test.p2p {
				t.Fatalf("p2p = %v, want %v", resp.GetP2PState(), test.p2p)
			}
			if resp.GetLastError() != test.errMsg {
				t.Fatalf("last error = %q, want %q", resp.GetLastError(), test.errMsg)
			}
			if resp.GetUploadBytesPerSecond() != 0 || resp.GetDownloadBytesPerSecond() != 0 {
				t.Fatalf(
					"throughput = %d/%d, want no p2p throughput",
					resp.GetUploadBytesPerSecond(),
					resp.GetDownloadBytesPerSecond(),
				)
			}
		})
	}
}

func TestSyncStatusRateStateLimitsRateUpdates(t *testing.T) {
	t.Parallel()

	rate := &syncStatusRateState{}
	first := syncStatusFromSpacewaveTelemetry(
		provider_spacewave.SyncTelemetrySnapshot{
			InFlightPushes: 1,
			PushedBytes:    0,
		},
		provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		false,
		rate,
		time.Unix(30, 0),
	)
	if first.GetUploadBytesPerSecond() != 0 {
		t.Fatalf("initial upload rate = %d, want 0", first.GetUploadBytesPerSecond())
	}

	limited := syncStatusFromSpacewaveTelemetry(
		provider_spacewave.SyncTelemetrySnapshot{
			InFlightPushes: 1,
			PushedBytes:    512,
		},
		provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		false,
		rate,
		time.Unix(30, int64(500*time.Millisecond)),
	)
	if limited.GetUploadBytesPerSecond() != 0 {
		t.Fatalf("limited upload rate = %d, want previous 0", limited.GetUploadBytesPerSecond())
	}

	updated := syncStatusFromSpacewaveTelemetry(
		provider_spacewave.SyncTelemetrySnapshot{
			InFlightPushes: 1,
			PushedBytes:    1024,
		},
		provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		false,
		rate,
		time.Unix(31, 0),
	)
	if updated.GetUploadBytesPerSecond() != 1024 {
		t.Fatalf("updated upload rate = %d, want 1024", updated.GetUploadBytesPerSecond())
	}
}

func TestSyncStatusRateStateIncludesActiveUploadProgress(t *testing.T) {
	t.Parallel()

	rate := &syncStatusRateState{}
	_ = syncStatusFromSpacewaveTelemetry(
		provider_spacewave.SyncTelemetrySnapshot{
			InFlightPushes:               1,
			ActiveUploadBytes:            4096,
			ActiveUploadTransferredBytes: 0,
		},
		provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		false,
		rate,
		time.Unix(40, 0),
	)

	resp := syncStatusFromSpacewaveTelemetry(
		provider_spacewave.SyncTelemetrySnapshot{
			InFlightPushes:               1,
			ActiveUploadBytes:            4096,
			ActiveUploadTransferredBytes: 2048,
		},
		provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		false,
		rate,
		time.Unix(41, 0),
	)
	if resp.GetUploadBytesPerSecond() != 2048 {
		t.Fatalf("active upload rate = %d, want 2048", resp.GetUploadBytesPerSecond())
	}
}

func TestSyncStatusSpacewavePackTelemetryFields(t *testing.T) {
	t.Parallel()

	resp := syncStatusFromSpacewaveTelemetry(
		provider_spacewave.SyncTelemetrySnapshot{
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
			IndexTailFetchCount:       9,
			IndexTailFetchBytes:       2048,
			IndexTailResponseBytes:    1024,
		},
		provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		false,
		&syncStatusRateState{},
		time.Unix(40, 0),
	)
	if resp.GetPackRangeRequestCount() != 3 || resp.GetPackRangeResponseBytes() != 3072 {
		t.Fatalf("unexpected range telemetry: %+v", resp)
	}
	if resp.GetPackManifestEntries() != 2 ||
		resp.GetPackBlockCountTotal() != 12 ||
		resp.GetPackBlockCountMin() != 2 ||
		resp.GetPackBlockCountMax() != 10 {
		t.Fatalf("unexpected manifest telemetry: %+v", resp)
	}
	if resp.GetPackBloomMaxFalsePositiveRate() != 0.02 || resp.GetPackBloomRiskPackCount() != 1 {
		t.Fatalf("unexpected bloom telemetry: %+v", resp)
	}
	if resp.GetPackLookupCount() != 4 ||
		resp.GetPackCandidatePacks() != 9 ||
		resp.GetPackOpenedPacks() != 5 ||
		resp.GetPackNegativePacks() != 3 ||
		resp.GetPackTargetHits() != 1 ||
		!resp.GetPackLastTargetHit() {
		t.Fatalf("unexpected lookup telemetry: %+v", resp)
	}
	if resp.GetPackIndexCacheHits() != 6 ||
		resp.GetPackIndexCacheMisses() != 7 ||
		resp.GetPackRemoteIndexLoads() != 8 ||
		resp.GetPackRemoteIndexBytes() != 8192 ||
		resp.GetPackLastRemoteIndexBytes() != 512 ||
		resp.GetPackIndexTailFetchCount() != 9 ||
		resp.GetPackIndexTailFetchBytes() != 2048 ||
		resp.GetPackIndexTailResponseBytes() != 1024 {
		t.Fatalf("unexpected index telemetry: %+v", resp)
	}
}

func recvSyncStatusResponse(
	t *testing.T,
	msgs <-chan *s4wave_session.WatchSyncStatusResponse,
) *s4wave_session.WatchSyncStatusResponse {
	t.Helper()
	select {
	case resp := <-msgs:
		return resp
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for sync status response")
		return nil
	}
}

// _ is a type assertion
var (
	_ s4wave_session.SRPCSessionResourceService_WatchSyncStatusStream = (*testWatchSyncStatusStream)(nil)
	_ session.Session                                                 = (*testSyncStatusSession)(nil)
)
