//go:build !js

package spacewave_cli

import (
	"context"
	"strconv"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"

	core_session "github.com/s4wave/spacewave/core/session"
)

// syncKeepaliveRetryDelay is the pause before rewatching core after the
// session list stream ends, such as when a Dist core plugin restarts.
const syncKeepaliveRetryDelay = time.Second

// startSyncKeepalive holds the daemon idle tracker while a session has uploads
// pending. A short CLI write leaves its edits pending until the next sync
// flush; the daemon outlives that flush so the edits reach the cloud. It
// watches core through the Resource invoker, so native and Dist core behave
// the same.
func startSyncKeepalive(
	ctx context.Context,
	le *logrus.Entry,
	invoker srpc.Invoker,
	idleTracker *daemonIdleTracker,
) {
	go func() {
		for {
			err := watchSyncKeepalive(ctx, le, invoker, idleTracker)
			if ctx.Err() != nil {
				return
			}
			le.WithError(err).Debug("sync keepalive watch ended, retrying")
			select {
			case <-ctx.Done():
				return
			case <-time.After(syncKeepaliveRetryDelay):
			}
		}
	}()
}

// watchSyncKeepalive follows the session list and runs one sync watch per
// listed session until the list stream ends.
func watchSyncKeepalive(
	ctx context.Context,
	le *logrus.Entry,
	invoker srpc.Invoker,
	idleTracker *daemonIdleTracker,
) error {
	// Build the SDK client and watch the daemon's session list.
	client, err := buildSDKClientFromInvoker(ctx, invoker)
	if err != nil {
		return err
	}
	defer client.close()
	watch, err := client.root.WatchSessions(ctx)
	if err != nil {
		return err
	}
	defer watch.Close()

	// Stop every session watch on exit.
	watches := make(map[uint32]func())
	defer func() {
		for _, stop := range watches {
			stop()
		}
	}()
	for {
		resp, err := watch.Recv()
		if err != nil {
			return err
		}
		reconcileSyncKeepaliveWatches(ctx, le, client, resp.GetSessions(), watches, idleTracker)
	}
}

// reconcileSyncKeepaliveWatches starts a sync watch for each new session and
// stops the watches of sessions that are gone.
func reconcileSyncKeepaliveWatches(
	ctx context.Context,
	le *logrus.Entry,
	client *sdkClient,
	entries []*core_session.SessionListEntry,
	watches map[uint32]func(),
	idleTracker *daemonIdleTracker,
) {
	// Start a watch for each listed session without one.
	present := make(map[uint32]struct{}, len(entries))
	for _, entry := range entries {
		index := entry.GetSessionIndex()
		present[index] = struct{}{}
		if _, ok := watches[index]; ok {
			continue
		}

		// Run the watch until the session leaves the list.
		watchCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			err := watchSessionUploads(watchCtx, client, index, idleTracker)
			if watchCtx.Err() == nil {
				le.WithError(err).WithField("session-index", index).Debug("session sync keepalive ended")
			}
		}()
		watches[index] = func() {
			cancel()
			<-done
		}
	}

	// Stop the watches of sessions removed from the list.
	for index, stop := range watches {
		if _, ok := present[index]; ok {
			continue
		}
		stop()
		delete(watches, index)
	}
}

// watchSessionUploads holds the idle tracker while the session reports
// pending or in-flight uploads. An upload error releases the hold: the
// provider keeps the durable work for the next run. Download and transport
// errors do not, since the uploads may still succeed.
func watchSessionUploads(
	ctx context.Context,
	client *sdkClient,
	index uint32,
	idleTracker *daemonIdleTracker,
) error {
	// Mount the session and watch its sync status.
	sess, err := client.mountSession(ctx, index)
	if err != nil {
		return err
	}
	defer sess.Release()
	strm, err := sess.WatchSyncStatus(ctx)
	if err != nil {
		return err
	}
	defer strm.Close()

	// Hold the daemon while uploads are pending, releasing on exit.
	var release func()
	defer func() {
		if release != nil {
			release()
		}
	}()
	for {
		status, err := strm.Recv()
		if err != nil {
			return err
		}

		// Take or release the hold as uploads appear and settle.
		pending := status.GetUploadError() == "" &&
			(status.GetPendingUploadCount() != 0 || status.GetPendingUploadBytes() != 0 || status.GetInFlightUploadCount() != 0)
		switch {
		case pending && release == nil:
			release = idleTracker.serviceAttached("uploads for session " + strconv.FormatUint(uint64(index), 10))
		case !pending && release != nil:
			release()
			release = nil
		}
	}
}
