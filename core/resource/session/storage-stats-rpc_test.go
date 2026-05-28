package resource_session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/db/volume"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

type testWatchStorageStatsStream struct {
	srpc.Stream
	ctx  context.Context
	msgs chan *s4wave_session.WatchStorageStatsResponse
}

func newTestWatchStorageStatsStream(ctx context.Context) *testWatchStorageStatsStream {
	return &testWatchStorageStatsStream{
		ctx:  ctx,
		msgs: make(chan *s4wave_session.WatchStorageStatsResponse, 4),
	}
}

func (m *testWatchStorageStatsStream) Context() context.Context {
	return m.ctx
}

func (m *testWatchStorageStatsStream) Send(resp *s4wave_session.WatchStorageStatsResponse) error {
	select {
	case m.msgs <- resp:
		return nil
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
}

func (m *testWatchStorageStatsStream) SendAndClose(resp *s4wave_session.WatchStorageStatsResponse) error {
	return m.Send(resp)
}

func (m *testWatchStorageStatsStream) MsgRecv(_ srpc.Message) error {
	return nil
}

func (m *testWatchStorageStatsStream) MsgSend(_ srpc.Message) error {
	return nil
}

func (m *testWatchStorageStatsStream) CloseSend() error {
	return nil
}

func (m *testWatchStorageStatsStream) Close() error {
	return nil
}

type testStorageStatsAccount struct {
	mtx    sync.Mutex
	stats  *volume.StorageStats
	waitCh chan struct{}
}

func newTestStorageStatsAccount(stats *volume.StorageStats) *testStorageStatsAccount {
	return &testStorageStatsAccount{
		stats:  stats,
		waitCh: make(chan struct{}),
	}
}

func (a *testStorageStatsAccount) GetProviderAccountFeature(
	context.Context,
	provider.ProviderFeature,
) (provider.ProviderAccountFeature, error) {
	return nil, errors.New("not implemented")
}

func (a *testStorageStatsAccount) GetStorageStats(context.Context) (*volume.StorageStats, error) {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return a.stats.CloneVT(), nil
}

func (a *testStorageStatsAccount) GetStorageStatsSnapshotWithWait(
	context.Context,
) (*volume.StorageStats, <-chan struct{}, error) {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return a.stats.CloneVT(), a.waitCh, nil
}

func (a *testStorageStatsAccount) setStats(stats *volume.StorageStats) {
	a.mtx.Lock()
	waitCh := a.waitCh
	a.stats = stats
	a.waitCh = make(chan struct{})
	a.mtx.Unlock()
	close(waitCh)
}

type testUnsupportedStorageStatsAccount struct{}

func (a *testUnsupportedStorageStatsAccount) GetProviderAccountFeature(
	context.Context,
	provider.ProviderFeature,
) (provider.ProviderAccountFeature, error) {
	return nil, errors.New("not implemented")
}

func TestWatchStorageStatsUnsupportedProvider(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	strm := newTestWatchStorageStatsStream(ctx)
	res := &SessionResource{
		session: &testSyncStatusSession{acc: &testUnsupportedStorageStatsAccount{}},
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- res.WatchStorageStats(&s4wave_session.WatchStorageStatsRequest{}, strm)
	}()

	resp := recvStorageStatsResponse(t, strm.msgs)
	if resp.GetSupported() {
		t.Fatalf("supported = true, want false")
	}

	cancel()
	if err := <-errCh; err != context.Canceled {
		t.Fatalf("WatchStorageStats() = %v, want context canceled", err)
	}
}

func TestWatchStorageStatsEmitsInitialAndChangedSnapshots(t *testing.T) {
	t.Parallel()

	acc := newTestStorageStatsAccount(&volume.StorageStats{TotalBytes: 1024, BlockCount: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	strm := newTestWatchStorageStatsStream(ctx)
	res := &SessionResource{
		session: &testSyncStatusSession{acc: acc},
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- res.WatchStorageStats(&s4wave_session.WatchStorageStatsRequest{}, strm)
	}()

	resp := recvStorageStatsResponse(t, strm.msgs)
	if !resp.GetSupported() || resp.GetTotalBytes() != 1024 || resp.GetBlockCount() != 2 {
		t.Fatalf("initial stats = %+v, want supported 1024/2", resp)
	}

	acc.setStats(&volume.StorageStats{TotalBytes: 1024, BlockCount: 2})
	assertNoStorageStatsResponse(t, strm.msgs)

	acc.setStats(&volume.StorageStats{TotalBytes: 2048, BlockCount: 3})
	resp = recvStorageStatsResponse(t, strm.msgs)
	if !resp.GetSupported() || resp.GetTotalBytes() != 2048 || resp.GetBlockCount() != 3 {
		t.Fatalf("changed stats = %+v, want supported 2048/3", resp)
	}

	cancel()
	if err := <-errCh; err != context.Canceled {
		t.Fatalf("WatchStorageStats() = %v, want context canceled", err)
	}
}

func recvStorageStatsResponse(
	t *testing.T,
	msgs <-chan *s4wave_session.WatchStorageStatsResponse,
) *s4wave_session.WatchStorageStatsResponse {
	t.Helper()
	select {
	case resp := <-msgs:
		return resp
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for storage stats response")
		return nil
	}
}

func assertNoStorageStatsResponse(
	t *testing.T,
	msgs <-chan *s4wave_session.WatchStorageStatsResponse,
) {
	t.Helper()
	select {
	case resp := <-msgs:
		t.Fatalf("unexpected storage stats response: %+v", resp)
	case <-time.After(50 * time.Millisecond):
	}
}

var (
	_ provider.ProviderAccount                                          = (*testStorageStatsAccount)(nil)
	_ provider.StorageStatsWatchProvider                                = (*testStorageStatsAccount)(nil)
	_ provider.ProviderAccount                                          = (*testUnsupportedStorageStatsAccount)(nil)
	_ s4wave_session.SRPCSessionResourceService_WatchStorageStatsStream = (*testWatchStorageStatsStream)(nil)
)
