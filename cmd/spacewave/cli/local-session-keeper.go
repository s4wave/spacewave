//go:build !js

package spacewave_cli

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	core_session "github.com/s4wave/spacewave/core/session"
	"github.com/sirupsen/logrus"
)

// localSessionMount retains a session resource until the keeper releases it.
type localSessionMount interface {
	Release()
}

// startLocalSessionKeeper keeps configured local sessions mounted while the
// daemon serves requests. Their session transports and P2P controllers must
// remain available after a short CLI request releases its own resource.
func startLocalSessionKeeper(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
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
		var enrollmentCleanup func()
		defer func() {
			if enrollmentCleanup != nil {
				enrollmentCleanup()
			}
			for _, sess := range mounted {
				sess.Release()
			}
		}()
		mount := func(index uint32) (localSessionMount, error) {
			return client.mountSession(ctx, index)
		}
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
				mount,
			)
			reconcileDeviceEnrollment(ctx, le, statePath, client, resp.GetSessions(), mount, &enrollmentCleanup)
		}
	}()
}

// reconcileDeviceEnrollment restores a persisted local Device mount and retries
// pending World projection only when the recorded session appears in the list.
func reconcileDeviceEnrollment(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	client *sdkClient,
	entries []*core_session.SessionListEntry,
	mount func(uint32) (localSessionMount, error),
	enrollmentCleanup *func(),
) {
	if *enrollmentCleanup == nil {
		cleanup, err := restoreLocalDeviceEnrollment(ctx, statePath, client, mount)
		if err != nil {
			le.WithError(err).Warn("local Device enrollment restore failed")
			return
		}
		*enrollmentCleanup = cleanup
	}

	record, err := readDeviceSetupRecord(statePath)
	if err != nil {
		le.WithError(err).Warn("Device setup state unavailable")
		return
	}
	if record.SetupState != deviceSetupStateImported || record.SessionIndex == 0 || record.DeviceObjectKey != "" {
		return
	}
	for _, entry := range entries {
		if entry.GetSessionIndex() != record.SessionIndex {
			continue
		}
		if err := projectPendingDeviceEnrollment(ctx, statePath, client); err != nil {
			le.WithError(err).Warn("Device object projection pending")
		}
		return
	}
}

// reconcileLocalSessionMounts keeps local sessions in the latest list mounted
// and releases mounts removed from that list.
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
