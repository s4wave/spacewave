package volume_controller

import (
	"context"
	"errors"
	"testing"
	"time"

	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/volume"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestExecuteStopsOnPermanentError proves a permanent construction failure ends
// Execute without an error (so controllerbus does not restart the controller)
// and that GetVolume returns the failure instead of blocking forever.
func TestExecuteStopsOnPermanentError(t *testing.T) {
	le := logrus.NewEntry(logrus.New())
	cause := errors.New("opfs GetRoot: SecurityError")
	wantErr := volume.Permanent(cause)

	ctrl := NewController(le, &Config{}, nil, nil,
		func(ctx context.Context, le *logrus.Entry) (volume.Volume, error) {
			return nil, wantErr
		})

	if err := ctrl.Execute(context.Background()); err != nil {
		t.Fatalf("Execute returned %v, want nil so controllerbus does not retry", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	vol, err := ctrl.GetVolume(ctx)
	if vol != nil {
		t.Fatalf("GetVolume returned a volume, want nil")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("GetVolume returned %v, want it to wrap %v", err, cause)
	}
	if !volume.IsPermanent(err) {
		t.Fatalf("GetVolume error %v is not classified permanent", err)
	}
}

// TestExecuteRetriesTransientError proves a non-permanent construction failure
// still propagates out of Execute so controllerbus restarts with backoff.
func TestExecuteRetriesTransientError(t *testing.T) {
	le := logrus.NewEntry(logrus.New())
	transient := errors.New("temporary failure")

	ctrl := NewController(le, &Config{}, nil, nil,
		func(ctx context.Context, le *logrus.Entry) (volume.Volume, error) {
			return nil, transient
		})

	if err := ctrl.Execute(context.Background()); !errors.Is(err, transient) {
		t.Fatalf("Execute returned %v, want the transient error so it retries", err)
	}
	if got := ctrl.getTerminal(); got != nil {
		t.Fatalf("getTerminal returned %v, want nil for a transient error", got)
	}
}

type joinedExecuteVolume struct {
	volume.Volume
	executeStarted chan struct{}
	executeExited  chan struct{}
	closeErr       chan error
	getPeerErr     error
}

func (v *joinedExecuteVolume) Execute(ctx context.Context) error {
	close(v.executeStarted)
	<-ctx.Done()
	close(v.executeExited)
	return ctx.Err()
}

func (v *joinedExecuteVolume) GetPeer(ctx context.Context, withPriv bool) (peer.Peer, error) {
	if v.getPeerErr != nil {
		return nil, v.getPeerErr
	}
	return v.Volume.GetPeer(ctx, withPriv)
}

func (v *joinedExecuteVolume) Close() error {
	select {
	case <-v.executeExited:
		v.closeErr <- nil
	default:
		v.closeErr <- errors.New("Close overlapped Execute")
	}
	return v.Volume.Close()
}

func TestExecuteJoinsVolumeBeforeClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	base, err := common_kvtx.NewVolume(
		ctx,
		"test-volume",
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		nil,
		false,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	vol := &joinedExecuteVolume{
		Volume:         base,
		executeStarted: make(chan struct{}),
		executeExited:  make(chan struct{}),
		closeErr:       make(chan error, 1),
	}
	ctrl := NewController(
		logrus.NewEntry(logrus.New()),
		&Config{DisablePeer: true},
		nil,
		nil,
		func(context.Context, *logrus.Entry) (volume.Volume, error) { return vol, nil },
	)
	done := make(chan error, 1)
	go func() { done <- ctrl.Execute(ctx) }()

	select {
	case <-vol.executeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("volume Execute did not start")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("controller Execute error = %v, want context.Canceled", err)
	}
	if err := <-vol.closeErr; err != nil {
		t.Fatal(err)
	}
}

func TestExecuteJoinsVolumeBeforeCloseOnInitializationError(t *testing.T) {
	ctx := context.Background()
	base, err := common_kvtx.NewVolume(
		ctx,
		"initialization-error-volume",
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		nil,
		false,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	getPeerErr := errors.New("get peer failed")
	vol := &joinedExecuteVolume{
		Volume:         base,
		executeStarted: make(chan struct{}),
		executeExited:  make(chan struct{}),
		closeErr:       make(chan error, 1),
		getPeerErr:     getPeerErr,
	}
	ctrl := NewController(
		logrus.NewEntry(logrus.New()),
		&Config{},
		nil,
		nil,
		func(context.Context, *logrus.Entry) (volume.Volume, error) { return vol, nil },
	)
	if err := ctrl.Execute(ctx); !errors.Is(err, getPeerErr) {
		t.Fatalf("controller Execute error = %v, want %v", err, getPeerErr)
	}
	if err := <-vol.closeErr; err != nil {
		t.Fatal(err)
	}
}

func TestExecuteKeepsNilReturnVolumeReadyUntilCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	base, err := common_kvtx.NewVolume(
		ctx,
		"nil-return-volume",
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		nil,
		false,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	ctrl := NewController(
		logrus.NewEntry(logrus.New()),
		&Config{DisablePeer: true},
		nil,
		nil,
		func(context.Context, *logrus.Entry) (volume.Volume, error) { return base, nil },
	)
	done := make(chan error, 1)
	go func() { done <- ctrl.Execute(ctx) }()

	readyCtx, readyCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer readyCancel()
	if got, err := ctrl.GetVolume(readyCtx); err != nil {
		t.Fatal(err)
	} else if got != base {
		t.Fatal("GetVolume returned a different volume")
	}
	select {
	case err := <-done:
		t.Fatalf("controller returned before cancellation: %v", err)
	default:
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("controller Execute error = %v, want context.Canceled", err)
	}
}
