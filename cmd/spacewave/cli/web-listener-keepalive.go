//go:build !js

package spacewave_cli

import (
	"context"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"

	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

// webListenerKeepaliveRetryDelay is the pause before rewatching core after the
// web listener stream ends, such as when a Dist core plugin restarts.
const webListenerKeepaliveRetryDelay = time.Second

// startWebListenerKeepalive holds the daemon idle tracker while core serves
// background web listeners. It watches core through the Resource invoker, so
// native and Dist core behave the same.
func startWebListenerKeepalive(
	ctx context.Context,
	le *logrus.Entry,
	invoker srpc.Invoker,
	idleTracker *daemonIdleTracker,
) {
	go func() {
		for {
			err := watchWebListenerKeepalive(ctx, invoker, idleTracker)
			if ctx.Err() != nil {
				return
			}
			le.WithError(err).Debug("web listener keepalive watch ended, retrying")
			select {
			case <-ctx.Done():
				return
			case <-time.After(webListenerKeepaliveRetryDelay):
			}
		}
	}()
}

// watchWebListenerKeepalive holds one idle tracker release per listed web
// listener until the watch ends, then releases all of them.
func watchWebListenerKeepalive(
	ctx context.Context,
	invoker srpc.Invoker,
	idleTracker *daemonIdleTracker,
) error {
	client, err := buildSDKClientFromInvoker(ctx, invoker)
	if err != nil {
		return err
	}
	defer client.close()

	watch, err := client.root.WatchWebListeners(ctx)
	if err != nil {
		return err
	}
	defer watch.Close()

	held := make(map[string]func())
	defer func() {
		for _, release := range held {
			release()
		}
	}()
	for {
		resp, err := watch.Recv()
		if err != nil {
			return err
		}
		reconcileWebListenerHolds(resp.GetListeners(), held, idleTracker)
	}
}

// reconcileWebListenerHolds acquires holds for new listeners and releases
// holds for listeners that are gone.
func reconcileWebListenerHolds(
	listeners []*s4wave_root.WebListenerInfo,
	held map[string]func(),
	idleTracker *daemonIdleTracker,
) {
	present := make(map[string]struct{}, len(listeners))
	for _, info := range listeners {
		id := info.GetListenerId()
		present[id] = struct{}{}
		if _, ok := held[id]; !ok {
			held[id] = idleTracker.serviceAttached()
		}
	}
	for id, release := range held {
		if _, ok := present[id]; !ok {
			release()
			delete(held, id)
		}
	}
}
