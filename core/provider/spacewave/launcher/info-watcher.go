package spacewave_launcher

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// InfoWatcher mirrors the LauncherInfo of the launcher reachable on a bus.
// Desktop builds run the launcher in its own plugin process, so the watcher
// reads it through the Launcher RPC service rather than the controller. When
// the stream ends, the watcher clears its snapshot and looks the service up
// again with backoff.
type InfoWatcher struct {
	// b is the bus used to look up the Launcher service.
	b bus.Bus
	// rc runs the lookup and stream, retrying with backoff.
	rc *routine.RoutineContainer

	// bcast guards info.
	bcast broadcast.Broadcast
	// info is the latest LauncherInfo, or nil when no launcher is reachable.
	info *LauncherInfo
}

// NewInfoWatcher constructs an InfoWatcher for the launcher on b. It watches
// nothing until SetContext. le logs routine exits and may be nil.
func NewInfoWatcher(le *logrus.Entry, b bus.Bus) *InfoWatcher {
	opts := []routine.Option{
		routine.WithRetry(&backoff.Backoff{
			BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
			Exponential: &backoff.Exponential{
				InitialInterval: 500,
				MaxInterval:     10000,
			},
		}),
	}
	if le != nil {
		opts = append(opts, routine.WithExitLogger(le.WithField("routine", "launcher-info-watcher")))
	}

	w := &InfoWatcher{b: b}
	w.rc = routine.NewRoutineContainer(opts...)
	w.rc.SetRoutine(w.watch)
	return w
}

// SetContext watches the launcher until ctx is canceled.
func (w *InfoWatcher) SetContext(ctx context.Context) {
	_ = w.rc.SetContext(ctx, true)
}

// Snapshot returns a copy of the latest LauncherInfo, nil when no launcher is
// reachable, and a channel closed on the next change.
func (w *InfoWatcher) Snapshot() (*LauncherInfo, <-chan struct{}) {
	var info *LauncherInfo
	var waitCh <-chan struct{}
	w.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		info = w.info.CloneVT()
		waitCh = getWaitCh()
	})
	return info, waitCh
}

// watch streams LauncherInfo from the Launcher service until the stream ends.
func (w *InfoWatcher) watch(ctx context.Context) error {
	defer w.set(nil)

	// Wait for a Launcher service on the bus.
	invokers, _, invokerRef, err := bifrost_rpc.ExLookupRpcService(
		ctx,
		w.b,
		SRPCLauncherServiceID,
		"",
		true,
		func() {
			w.set(nil)
		},
	)
	if err != nil {
		return err
	}
	if len(invokers) == 0 {
		return errors.New("launcher service not found")
	}
	defer invokerRef.Release()

	// Mirror the launcher state until the stream ends.
	client := NewSRPCLauncherClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invokers[0]))))
	strm, err := client.WatchLauncherInfo(ctx, &WatchLauncherInfoRequest{})
	if err != nil {
		return err
	}
	defer strm.Close()
	for {
		info, err := strm.Recv()
		if err != nil {
			return err
		}
		w.set(info)
	}
}

// set replaces the snapshot and wakes waiters when it changed.
func (w *InfoWatcher) set(info *LauncherInfo) {
	w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if w.info.EqualVT(info) {
			return
		}
		w.info = info.CloneVT()
		broadcast()
	})
}

// _ is a type assertion
var _ routine.Routine = (*InfoWatcher)(nil).watch
