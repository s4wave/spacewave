//go:build !js

package spacewave_cli

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"

	core_session "github.com/s4wave/spacewave/core/session"
)

type localSessionMount interface {
	Release()
}

// startLocalSessionKeeper keeps configured local sessions mounted while the
// daemon serves requests. Their session transports and P2P controllers must
// remain available after a short CLI request releases its own resource.
func startLocalSessionKeeper(
	ctx context.Context,
	le *logrus.Entry,
	invoker srpc.Invoker,
) {
	if invoker == nil {
		return
	}
	go func() {
		client, err := buildSDKClientFromInvoker(ctx, invoker)
		if err != nil {
			if ctx.Err() == nil {
				le.WithError(err).Warn("local session keeper unavailable")
			}
			return
		}
		defer client.close()

		watch, err := client.root.WatchSessions(ctx)
		if err != nil {
			if ctx.Err() == nil {
				le.WithError(err).Warn("local session keeper could not watch sessions")
			}
			return
		}
		defer watch.Close()

		mounted := make(map[uint32]localSessionMount)
		defer func() {
			for _, sess := range mounted {
				sess.Release()
			}
		}()
		for {
			resp, err := watch.Recv()
			if err != nil {
				if ctx.Err() == nil {
					le.WithError(err).Warn("local session keeper stopped")
				}
				return
			}
			reconcileLocalSessionMounts(
				le,
				resp.GetSessions(),
				mounted,
				func(index uint32) (localSessionMount, error) {
					return client.mountSession(ctx, index)
				},
			)
		}
	}()
}

func reconcileLocalSessionMounts(
	le *logrus.Entry,
	entries []*core_session.SessionListEntry,
	mounted map[uint32]localSessionMount,
	mount func(uint32) (localSessionMount, error),
) {
	present := make(map[uint32]struct{})
	for _, entry := range entries {
		ref := entry.GetSessionRef()
		if ref.GetProviderResourceRef().GetProviderId() != "local" {
			continue
		}
		index := entry.GetSessionIndex()
		present[index] = struct{}{}
		if _, ok := mounted[index]; ok {
			continue
		}
		sess, err := mount(index)
		if err != nil {
			le.WithError(err).WithField("session-index", index).
				Warn("local session keeper could not mount session")
			continue
		}
		mounted[index] = sess
	}
	for index, sess := range mounted {
		if _, ok := present[index]; ok {
			continue
		}
		sess.Release()
		delete(mounted, index)
	}
}
