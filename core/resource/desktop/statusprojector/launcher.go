package statusprojector

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

type launcherInfoWatcher struct {
	bcast broadcast.Broadcast
	info  *spacewave_launcher.LauncherInfo
}

func newLauncherInfoWatcher(ctx context.Context, b bus.Bus) *launcherInfoWatcher {
	w := &launcherInfoWatcher{}
	go w.execute(ctx, b)
	return w
}

func (w *launcherInfoWatcher) Snapshot() (*spacewave_launcher.LauncherInfo, <-chan struct{}) {
	var info *spacewave_launcher.LauncherInfo
	var waitCh <-chan struct{}
	w.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		info = w.info.CloneVT()
		waitCh = getWaitCh()
	})
	return info, waitCh
}

func (w *launcherInfoWatcher) execute(ctx context.Context, b bus.Bus) {
	invokers, _, invokerRef, err := bifrost_rpc.ExLookupRpcService(
		ctx,
		b,
		spacewave_launcher.SRPCLauncherServiceID,
		"",
		true,
		func() {
			w.set(nil)
		},
	)
	if err != nil || len(invokers) == 0 {
		return
	}
	defer invokerRef.Release()

	client := spacewave_launcher.NewSRPCLauncherClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invokers[0]))))
	strm, err := client.WatchLauncherInfo(ctx, &spacewave_launcher.WatchLauncherInfoRequest{})
	if err != nil {
		return
	}
	defer strm.Close()

	for {
		info, err := strm.Recv()
		if err != nil {
			w.set(nil)
			return
		}
		w.set(info)
	}
}

func (w *launcherInfoWatcher) set(info *spacewave_launcher.LauncherInfo) {
	w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if w.info.EqualVT(info) {
			return
		}
		w.info = info.CloneVT()
		broadcast()
	})
}
