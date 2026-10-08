//go:build !js

package spacewave_cli

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	core_session "github.com/s4wave/spacewave/core/session"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/sirupsen/logrus"
)

// localSessionMount retains a session resource until the keeper releases it.
type localSessionMount interface {
	// Release drops the retained session.
	Release()
}

// keeperSessionMount retains one local Session and its binding watches.
type keeperSessionMount struct {
	// session is the retained SDK Session resource.
	session *s4wave_session.Session
	// cancel ends the Session's binding watches.
	cancel context.CancelFunc
	// done closes after all Space watches release their contents mounts.
	done <-chan struct{}
}

// Release ends all binding watches before dropping the Session mount.
func (m *keeperSessionMount) Release() {
	m.cancel()
	<-m.done
	m.session.Release()
}

// keeperSpaceWatch owns one Space binding stream and its contents reference.
type keeperSpaceWatch struct {
	// cancel ends the binding stream.
	cancel context.CancelFunc
	// done closes after the contents reference is released.
	done <-chan struct{}
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
		// Build the SDK client from the invoker.
		client, err := buildSDKClientFromInvoker(ctx, invoker)
		if err != nil {
			if ctx.Err() == nil {
				le.WithError(err).Warn("local session keeper unavailable")
			}
			return
		}
		defer client.close()

		// Watch the daemon's session list.
		watch, err := client.root.WatchSessions(ctx)
		if err != nil {
			if ctx.Err() == nil {
				le.WithError(err).Warn("local session keeper could not watch sessions")
			}
			return
		}
		defer watch.Close()

		// Release every mounted session and the enrollment cleanup on exit.
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
			// Mount a session with its own cancelable context.
			session, err := client.mountSession(ctx, index)
			if err != nil {
				return nil, err
			}
			sessionCtx, cancel := context.WithCancel(ctx)
			done := make(chan struct{})
			go func() {
				defer close(done)
				watchLocalSessionBindings(sessionCtx, le, client, session)
			}()
			return &keeperSessionMount{session: session, cancel: cancel, done: done}, nil
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
			reconcileDeviceEnrollment(ctx, le, statePath, client, mount, &enrollmentCleanup)
		}
	}()
}

// reconcileDeviceEnrollment restores a persisted local Device enrollment once
// its session can be mounted, and keeps it until the keeper exits.
func reconcileDeviceEnrollment(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	client *sdkClient,
	mount func(uint32) (localSessionMount, error),
	enrollmentCleanup *func(),
) {
	if *enrollmentCleanup != nil {
		return
	}
	cleanup, err := restoreLocalDeviceEnrollment(ctx, le, statePath, client, mount)
	if err != nil {
		le.WithError(err).Warn("local Device enrollment restore failed")
		return
	}
	*enrollmentCleanup = cleanup
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

// watchLocalSessionBindings follows the Spaces of a retained local Session.
// A Space watch owns its contents reference only while a process binding is
// approved, independently of every command's temporary contents mount.
func watchLocalSessionBindings(ctx context.Context, le *logrus.Entry, client *sdkClient, session *s4wave_session.Session) {
	// Watch the session's resource list for Space bindings.
	stream, err := session.WatchResourcesList(ctx)
	if err != nil {
		if ctx.Err() == nil {
			le.WithError(err).Warn("local session keeper could not watch Spaces")
		}
		return
	}
	defer stream.Close()

	// Release every Space watch on exit.
	spaces := make(map[string]*keeperSpaceWatch)
	defer func() {
		for _, watch := range spaces {
			watch.cancel()
		}
		for _, watch := range spaces {
			<-watch.done
		}
	}()
	for {
		resp, err := stream.Recv()
		if err != nil {
			if ctx.Err() == nil {
				le.WithError(err).Warn("local session keeper Space watch stopped")
			}
			return
		}
		ids := make([]string, 0, len(resp.GetSpacesList()))
		for _, entry := range resp.GetSpacesList() {
			spaceID := entry.GetEntry().GetRef().GetProviderResourceRef().GetId()
			if spaceID == "" {
				continue
			}
			ids = append(ids, spaceID)
		}
		reconcileLocalSpaceWatches(spaces, ids, func(spaceID string) *keeperSpaceWatch {
			spaceCtx, cancel := context.WithCancel(ctx)
			done := make(chan struct{})
			go func() {
				defer close(done)
				watchLocalSpaceBindings(spaceCtx, le, client, session, spaceID)
			}()
			return &keeperSpaceWatch{cancel: cancel, done: done}
		})
	}
}

// reconcileLocalSpaceWatches restarts stopped Space watches on Session
// resource-list revisions and releases watches absent from the latest list.
func reconcileLocalSpaceWatches(spaces map[string]*keeperSpaceWatch, ids []string, start func(string) *keeperSpaceWatch) {
	present := make(map[string]struct{}, len(ids))
	for _, spaceID := range ids {
		present[spaceID] = struct{}{}
		if watch, ok := spaces[spaceID]; ok {
			select {
			case <-watch.done:
				watch.cancel()
				delete(spaces, spaceID)
			default:
				continue
			}
		}
		spaces[spaceID] = start(spaceID)
	}
	for spaceID, watch := range spaces {
		if _, ok := present[spaceID]; ok {
			continue
		}
		watch.cancel()
		<-watch.done
		delete(spaces, spaceID)
	}
}

// watchLocalSpaceBindings retains the shared runtime while any process binding
// in this Space is approved for the mounted Session.
func watchLocalSpaceBindings(ctx context.Context, le *logrus.Entry, client *sdkClient, session *s4wave_session.Session, spaceID string) {
	space, releaseSpace, err := client.mountSpace(ctx, session, spaceID)
	if err != nil {
		if ctx.Err() == nil {
			le.WithError(err).WithField("space-id", spaceID).Warn("local session keeper could not mount Space")
		}
		return
	}
	defer releaseSpace()
	retainApprovedProcessRuntime(ctx, le, spaceID, space, func() (func(), error) {
		_, release, err := client.mountSpaceContents(ctx, space)
		return release, err
	})
}

// retainApprovedProcessRuntime follows local binding decisions and holds one
// shared runtime reference exactly while any process binding is approved. The
// Space runtime runs every approved process, so each approval needs the runtime
// mounted after the command that approved it releases its own mount.
func retainApprovedProcessRuntime(
	ctx context.Context,
	le *logrus.Entry,
	spaceID string,
	space s4wave_space.SRPCSpaceResourceServiceClient,
	mountContents func() (func(), error),
) {
	// Watch the Space's process bindings.
	stream, err := space.WatchProcessBindings(ctx, &s4wave_space.WatchProcessBindingsRequest{})
	if err != nil {
		if ctx.Err() == nil {
			le.WithError(err).WithField("space-id", spaceID).Warn("local session keeper could not watch bindings")
		}
		return
	}
	defer stream.Close()

	// Release the retained contents mount on exit.
	var releaseContents func()
	defer func() {
		if releaseContents != nil {
			releaseContents()
		}
	}()
	for {
		resp, err := stream.Recv()
		if err != nil {
			if ctx.Err() == nil {
				le.WithError(err).WithField("space-id", spaceID).Warn("local session keeper binding watch stopped")
			}
			return
		}
		approved := false
		for _, binding := range resp.GetProcessBindings() {
			if binding.GetApproved() {
				approved = true
				break
			}
		}
		if approved && releaseContents == nil {
			release, err := mountContents()
			if err != nil {
				le.WithError(err).WithField("space-id", spaceID).Warn("local session keeper could not retain Space runtime")
				return
			}
			releaseContents = release
		}
		if !approved && releaseContents != nil {
			releaseContents()
			releaseContents = nil
		}
	}
}
