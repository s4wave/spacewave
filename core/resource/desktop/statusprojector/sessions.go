package statusprojector

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/cdn"
	"github.com/s4wave/spacewave/core/provider"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/resource/desktop/statusprojector/activitypolicy"
	"github.com/s4wave/spacewave/core/resource/desktop/statusprojector/projection"
	"github.com/s4wave/spacewave/core/resource/desktop/statusprojector/spacepolicy"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

type sessionAccountProjection struct {
	status         provider.ProviderAccountStatus
	selfEnrollment *provider_spacewave.SelfEnrollmentProjection
}

type sessionRuntimeProjection struct {
	account sessionAccountProjection
	spaces  []*space.SpaceSoListEntry
	sync    *s4wave_session.WatchSyncStatusResponse
}

func snapshotSessionProjection(
	ctx context.Context,
	b bus.Bus,
	sessionCtrl session.SessionController,
) (*SessionProjection, []<-chan struct{}, []func(), error) {
	if sessionCtrl == nil {
		return &SessionProjection{}, nil, nil, nil
	}

	var sessionWaitCh <-chan struct{}
	sessionCtrl.GetSessionBroadcast().HoldLock(func(
		_ func(),
		getWaitCh func() <-chan struct{},
	) {
		sessionWaitCh = getWaitCh()
	})

	entries, err := sessionCtrl.ListSessions(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	rows := make([]*projection.SessionProjectionRow, 0, len(entries))
	spaceRows := []*spacepolicy.Row{}
	activityRows := []*activitypolicy.Row{}
	releases := make([]func(), 0, len(entries))
	waitChs := []<-chan struct{}{sessionWaitCh}
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		meta, err := sessionCtrl.GetSessionMetadata(ctx, entry.GetSessionIndex())
		if err != nil {
			return nil, nil, releases, err
		}
		runtime, runtimeWaitChs, runtimeReleases, err := snapshotSessionRuntimeProjection(ctx, b, entry)
		if err != nil {
			return nil, nil, releases, err
		}
		releases = append(releases, runtimeReleases...)
		waitChs = append(waitChs, runtimeWaitChs...)
		row := &projection.SessionProjectionRow{
			Entry:          entry,
			Metadata:       meta,
			AccountStatus:  runtime.account.status,
			SelfEnrollment: runtime.account.selfEnrollment,
		}
		rows = append(rows, row)
		label := projection.SessionLabel(row)
		for _, sp := range runtime.spaces {
			spaceRows = append(spaceRows, &spacepolicy.Row{
				SessionIndex: entry.GetSessionIndex(),
				SessionLabel: label,
				Space:        sp,
			})
		}
		if runtime.sync != nil {
			activityRows = append(activityRows, activityRowFromSyncStatus(entry.GetSessionIndex(), label, runtime.sync))
		}
	}

	proj := projection.BuildSessionProjection(rows)
	proj.Spaces = spacepolicy.Build(spaceRows)
	proj.Activity = activitypolicy.Build(activityRows)
	return proj, waitChs, releases, nil
}

func activityRowFromSyncStatus(
	sessionIndex uint32,
	sessionLabel string,
	status *s4wave_session.WatchSyncStatusResponse,
) *activitypolicy.Row {
	row := &activitypolicy.Row{
		SessionIndex:         sessionIndex,
		SessionLabel:         sessionLabel,
		PendingUploadCount:   uint64(status.GetPendingUploadCount()),
		PendingDownloadCount: uint64(status.GetPendingDownloadCount()),
		InFlightUploadCount:  uint64(status.GetInFlightUploadCount()),
		LastError:            status.GetLastError(),
	}
	switch status.GetState() {
	case s4wave_session.SyncStatusState_SyncStatusState_ERROR:
		row.State = activitypolicy.SyncStateError
	case s4wave_session.SyncStatusState_SyncStatusState_ACTIVE:
		row.State = activitypolicy.SyncStateActive
	case s4wave_session.SyncStatusState_SyncStatusState_SYNCED:
		row.State = activitypolicy.SyncStateSynced
	default:
		row.State = activitypolicy.SyncStateIdle
	}
	switch status.GetDirection() {
	case s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD:
		row.Direction = activitypolicy.SyncDirectionUpload
	case s4wave_session.SyncActivityDirection_SyncActivityDirection_DOWNLOAD:
		row.Direction = activitypolicy.SyncDirectionDownload
	case s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD_DOWNLOAD:
		row.Direction = activitypolicy.SyncDirectionUploadDownload
	default:
		row.Direction = activitypolicy.SyncDirectionUnknown
	}
	ts := status.GetLastActivityAt()
	if ts != nil && !ts.GetEmpty() {
		row.LastActivityAtUnixMs = ts.AsTime().UnixMilli()
		row.HasLastActivityAtTime = true
	}
	return row
}

func snapshotSessionRuntimeProjection(
	ctx context.Context,
	b bus.Bus,
	entry *session.SessionListEntry,
) (sessionRuntimeProjection, []<-chan struct{}, []func(), error) {
	sess, sessRef, err := session.ExMountSession(ctx, b, entry.GetSessionRef(), false, nil)
	if err != nil {
		return sessionRuntimeProjection{}, nil, nil, err
	}
	if sessRef == nil || sess == nil {
		return sessionRuntimeProjection{
			account: sessionAccountProjection{
				status: provider.ProviderAccountStatus_ProviderAccountStatus_NONE,
			},
		}, nil, nil, nil
	}

	releases := []func(){sessRef.Release}
	account := sess.GetProviderAccount()
	proj := sessionRuntimeProjection{
		account: sessionAccountProjection{
			status: provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		},
	}
	waitChs := []<-chan struct{}{}
	switch acc := account.(type) {
	case *provider_spacewave.ProviderAccount:
		accountProjection, accountWaitChs := snapshotSpacewaveSessionAccountProjection(acc)
		proj.account = accountProjection
		waitChs = append(waitChs, accountWaitChs...)
	}

	spaces, spaceWaitChs, spaceReleases, err := snapshotSessionSpaces(ctx, sess)
	if err != nil {
		releaseAll(spaceReleases)
		releaseAll(releases)
		return sessionRuntimeProjection{}, nil, nil, err
	}
	proj.spaces = spaces
	releases = append(releases, spaceReleases...)
	waitChs = append(waitChs, spaceWaitChs...)

	syncStatus, syncWaitChs := resource_session.BuildSyncStatusSnapshot(sess, time.Now())
	proj.sync = syncStatus
	waitChs = append(waitChs, syncWaitChs...)
	return proj, waitChs, releases, nil
}

func snapshotSpacewaveSessionAccountProjection(
	acc *provider_spacewave.ProviderAccount,
) (sessionAccountProjection, []<-chan struct{}) {
	proj := sessionAccountProjection{}
	acc.GetAccountBroadcast().HoldLock(func(
		_ func(),
		_ func() <-chan struct{},
	) {
		proj.status = acc.GetAccountStatus()
	})

	selfEnrollment, watch := acc.WatchSelfEnrollmentProjection()
	proj.selfEnrollment = selfEnrollment

	waitChs := make([]<-chan struct{}, 0, 3)
	appendWaitCh := func(ch <-chan struct{}) {
		if ch != nil {
			waitChs = append(waitChs, ch)
		}
	}
	appendWaitCh(watch.AccountCh)
	appendWaitCh(watch.RunCh)
	appendWaitCh(watch.EntityKeyCh)
	return proj, waitChs
}

func snapshotSessionSpaces(
	ctx context.Context,
	sess session.Session,
) ([]*space.SpaceSoListEntry, []<-chan struct{}, []func(), error) {
	providerAcc := sess.GetProviderAccount()
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, providerAcc)
	if err != nil {
		if errors.Is(err, provider.ErrUnimplementedProviderFeature) {
			return nil, nil, nil, nil
		}
		return nil, nil, nil, err
	}

	soListWatchable, releaseList, err := soProvider.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return nil, nil, nil, err
	}

	soList, spaces, err := spacepolicy.ReadSnapshot(soListWatchable, cdn.SpaceID())
	if err != nil {
		if releaseList != nil {
			releaseList()
		}
		return nil, nil, nil, err
	}

	watchCtx, cancel := context.WithCancel(ctx)
	waitCh := watchWatchableChange(watchCtx, soListWatchable, soList)
	return spaces, []<-chan struct{}{waitCh}, []func(){cancel, releaseList}, nil
}

func watchWatchableChange[T comparable](
	ctx context.Context,
	ctr ccontainer.Watchable[T],
	current T,
) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		_, _ = ctr.WaitValueChange(ctx, current, nil)
	}()
	return ch
}

func waitAnyStatusChange(ctx context.Context, waitChs []<-chan struct{}) bool {
	switch len(waitChs) {
	case 0:
		<-ctx.Done()
		return true
	case 1:
		select {
		case <-ctx.Done():
			return true
		case <-waitChs[0]:
			return false
		}
	}

	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan bool, 1)
	var once sync.Once
	for _, ch := range waitChs {
		go func(ch <-chan struct{}) {
			select {
			case <-waitCtx.Done():
			case <-ch:
				once.Do(func() {
					done <- false
					cancel()
				})
			}
		}(ch)
	}

	select {
	case <-ctx.Done():
		once.Do(func() {
			done <- true
			cancel()
		})
		return true
	case ctxDone := <-done:
		return ctxDone
	}
}

func releaseAll(releases []func()) {
	for _, v := range slices.Backward(releases) {
		if v != nil {
			v()
		}
	}
}
