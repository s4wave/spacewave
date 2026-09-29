//go:build !js

package spacewave_cli

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	core_session "github.com/s4wave/spacewave/core/session"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/sirupsen/logrus"
)

// localSessionMount retains a session resource until the keeper releases it.
type localSessionMount interface {
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

// watchLocalSessionBindings follows the Spaces of a retained local Session.
// A Space watch owns its contents reference only while a Forge Worker binding
// is approved, independently of every command's temporary contents mount.
func watchLocalSessionBindings(ctx context.Context, le *logrus.Entry, client *sdkClient, session *s4wave_session.Session) {
	stream, err := session.WatchResourcesList(ctx)
	if err != nil {
		if ctx.Err() == nil {
			le.WithError(err).Warn("local session keeper could not watch Spaces")
		}
		return
	}
	defer stream.Close()

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

// watchLocalSpaceBindings retains the shared runtime while any Forge Worker
// process binding in this Space is approved for the mounted Session.
func watchLocalSpaceBindings(ctx context.Context, le *logrus.Entry, client *sdkClient, session *s4wave_session.Session, spaceID string) {
	space, releaseSpace, err := client.mountSpace(ctx, session, spaceID)
	if err != nil {
		if ctx.Err() == nil {
			le.WithError(err).WithField("space-id", spaceID).Warn("local session keeper could not mount Space")
		}
		return
	}
	defer releaseSpace()
	retainApprovedForgeWorkerRuntime(ctx, le, spaceID, space, func() (func(), error) {
		_, release, err := client.mountSpaceContents(ctx, space)
		return release, err
	})
}

// retainApprovedForgeWorkerRuntime follows local binding decisions and holds
// one shared runtime reference exactly while a Forge Worker is approved.
func retainApprovedForgeWorkerRuntime(
	ctx context.Context,
	le *logrus.Entry,
	spaceID string,
	space s4wave_space.SRPCSpaceResourceServiceClient,
	mountContents func() (func(), error),
) {
	stream, err := space.WatchProcessBindings(ctx, &s4wave_space.WatchProcessBindingsRequest{})
	if err != nil {
		if ctx.Err() == nil {
			le.WithError(err).WithField("space-id", spaceID).Warn("local session keeper could not watch bindings")
		}
		return
	}
	defer stream.Close()

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
			if binding.GetTypeId() == forge_worker.WorkerTypeID && binding.GetApproved() {
				approved = true
				break
			}
		}
		if approved && releaseContents == nil {
			release, err := mountContents()
			if err != nil {
				le.WithError(err).WithField("space-id", spaceID).Warn("local session keeper could not retain Forge Worker runtime")
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
