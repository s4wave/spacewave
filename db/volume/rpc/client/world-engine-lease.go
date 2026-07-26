package volume_rpc_client

import (
	"context"
	"sync"
	"time"

	"github.com/s4wave/spacewave/db/volume"
	volume_rpc "github.com/s4wave/spacewave/db/volume/rpc"
)

const worldEngineLeaseReleaseTimeout = time.Second

type worldEngineLeaseProvider struct {
	client volume_rpc.SRPCProxyVolumeClient
}

func newWorldEngineLeaseProvider(client volume_rpc.SRPCProxyVolumeClient) volume.WorldEngineLeaseProvider {
	return &worldEngineLeaseProvider{client: client}
}

func (p *worldEngineLeaseProvider) AcquireWorldEngineLease(
	ctx context.Context,
	key string,
) (volume.WorldEngineLease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	leaseCtx, cancel := context.WithCancel(context.Background())
	acquisitionDone := make(chan struct{})
	var state struct {
		sync.Mutex
		acquired bool
		canceled bool
	}
	go func() {
		select {
		case <-ctx.Done():
			state.Lock()
			state.canceled = true
			shouldCancel := !state.acquired
			state.Unlock()
			if shouldCancel {
				cancel()
			}
		case <-acquisitionDone:
		}
	}()
	finishAcquisition := func(held bool) bool {
		state.Lock()
		if ctx.Err() != nil {
			state.canceled = true
		}
		if held && !state.canceled {
			state.acquired = true
		}
		acquired := state.acquired
		close(acquisitionDone)
		state.Unlock()
		if !acquired {
			cancel()
		}
		return acquired
	}

	stream, err := p.client.TryAcquireWorldEngineLease(
		leaseCtx,
		&volume_rpc.TryAcquireWorldEngineLeaseRequest{Key: key},
	)
	if err != nil {
		finishAcquisition(false)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}

	response, err := stream.Recv()
	if err != nil {
		finishAcquisition(false)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
	if !response.GetAcquired() {
		finishAcquisition(false)
		return nil, &volume.WorldEngineLeaseHeldError{Key: key}
	}
	if !finishAcquisition(true) {
		return nil, context.Canceled
	}

	lease := &worldEngineLease{
		client:  p.client,
		cancel:  cancel,
		leaseID: response.GetLeaseId(),
		done:    make(chan struct{}),
	}
	go lease.watchStream(stream)
	return lease, nil
}

type worldEngineLease struct {
	client     volume_rpc.SRPCProxyVolumeClient
	cancel     context.CancelFunc
	leaseID    string
	done       chan struct{}
	once       sync.Once
	doneOnce   sync.Once
	stateMtx   sync.Mutex
	released   bool
	lossErr    error
	releaseErr error
}

func (l *worldEngineLease) Done() <-chan struct{} {
	return l.done
}

func (l *worldEngineLease) Err() error {
	l.stateMtx.Lock()
	defer l.stateMtx.Unlock()
	return l.lossErr
}

func (l *worldEngineLease) watchStream(
	stream volume_rpc.SRPCProxyVolume_TryAcquireWorldEngineLeaseClient,
) {
	for {
		if _, err := stream.Recv(); err != nil {
			l.markLost(err)
			return
		}
	}
}

func (l *worldEngineLease) markLost(err error) {
	l.stateMtx.Lock()
	if l.released {
		l.stateMtx.Unlock()
		return
	}
	l.lossErr = err
	l.stateMtx.Unlock()
	l.doneOnce.Do(func() { close(l.done) })
	l.cancel()
}

func (l *worldEngineLease) Release() error {
	l.once.Do(func() {
		l.stateMtx.Lock()
		l.released = true
		lost := l.lossErr != nil
		l.stateMtx.Unlock()
		l.doneOnce.Do(func() { close(l.done) })
		l.cancel()
		if lost {
			return
		}

		releaseCtx, cancel := context.WithTimeout(context.Background(), worldEngineLeaseReleaseTimeout)
		defer cancel()
		releaseDone := make(chan error, 1)
		go func() {
			_, err := l.client.ReleaseWorldEngineLease(
				releaseCtx,
				&volume_rpc.ReleaseWorldEngineLeaseRequest{LeaseId: l.leaseID},
			)
			releaseDone <- err
		}()

		select {
		case l.releaseErr = <-releaseDone:
		case <-releaseCtx.Done():
			l.releaseErr = releaseCtx.Err()
		}
	})
	return l.releaseErr
}

var _ volume.WorldEngineLeaseProvider = (*worldEngineLeaseProvider)(nil)
var _ volume.WorldEngineLease = (*worldEngineLease)(nil)
