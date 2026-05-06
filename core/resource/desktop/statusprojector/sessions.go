package statusprojector

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/core/provider"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/session"
)

type sessionAccountProjection struct {
	status         provider.ProviderAccountStatus
	selfEnrollment *sessionSelfEnrollmentProjection
}

type sessionSelfEnrollmentProjection struct {
	count              uint32
	credentialRequired bool
	running            bool
	skipped            bool
	failed             bool
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

	rows := make([]*sessionProjectionRow, 0, len(entries))
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
		account, accountWaitChs, release, err := snapshotSessionAccountProjection(ctx, b, entry)
		if err != nil {
			return nil, nil, releases, err
		}
		if release != nil {
			releases = append(releases, release)
		}
		waitChs = append(waitChs, accountWaitChs...)
		rows = append(rows, &sessionProjectionRow{
			entry:          entry,
			metadata:       meta,
			accountStatus:  account.status,
			selfEnrollment: account.selfEnrollment,
		})
	}

	return buildSessionProjection(rows), waitChs, releases, nil
}

func snapshotSessionAccountProjection(
	ctx context.Context,
	b bus.Bus,
	entry *session.SessionListEntry,
) (sessionAccountProjection, []<-chan struct{}, func(), error) {
	sess, sessRef, err := session.ExMountSession(ctx, b, entry.GetSessionRef(), false, nil)
	if err != nil {
		return sessionAccountProjection{}, nil, nil, err
	}
	if sessRef == nil || sess == nil {
		return sessionAccountProjection{
			status: provider.ProviderAccountStatus_ProviderAccountStatus_NONE,
		}, nil, nil, nil
	}

	account := sess.GetProviderAccount()
	switch acc := account.(type) {
	case *provider_spacewave.ProviderAccount:
		proj, waitChs := snapshotSpacewaveSessionAccountProjection(acc)
		return proj, waitChs, sessRef.Release, nil
	default:
		return sessionAccountProjection{
			status: provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		}, nil, sessRef.Release, nil
	}
}

func snapshotSpacewaveSessionAccountProjection(
	acc *provider_spacewave.ProviderAccount,
) (sessionAccountProjection, []<-chan struct{}) {
	proj := sessionAccountProjection{}
	var accountCh <-chan struct{}
	var summary *provider_spacewave.SelfEnrollmentSummary
	var skippedKey string
	acc.GetAccountBroadcast().HoldLock(func(
		_ func(),
		getWaitCh func() <-chan struct{},
	) {
		accountCh = getWaitCh()
		proj.status = acc.GetAccountStatus()
		summary = acc.GetSelfEnrollmentSummary()
		skippedKey = acc.GetSelfEnrollmentSkippedGenerationKey()
	})

	run, runCh := acc.WatchSelfEnrollmentRunSnapshot()
	store := acc.GetEntityKeyStore()
	var entityCh <-chan struct{}
	unlockedCount := 0
	if store != nil {
		unlockedCount, entityCh = store.WatchUnlockedCount()
	}
	proj.selfEnrollment = buildSessionSelfEnrollmentProjection(
		summary,
		run,
		skippedKey,
		store != nil,
		unlockedCount,
	)

	waitChs := make([]<-chan struct{}, 0, 3)
	appendWaitCh := func(ch <-chan struct{}) {
		if ch != nil {
			waitChs = append(waitChs, ch)
		}
	}
	appendWaitCh(accountCh)
	appendWaitCh(runCh)
	appendWaitCh(entityCh)
	return proj, waitChs
}

func buildSessionSelfEnrollmentProjection(
	summary *provider_spacewave.SelfEnrollmentSummary,
	run *provider_spacewave.SelfEnrollmentRunSnapshot,
	skippedKey string,
	hasEntityKeyStore bool,
	unlockedCount int,
) *sessionSelfEnrollmentProjection {
	if summary == nil && run == nil {
		return nil
	}
	proj := &sessionSelfEnrollmentProjection{}
	if summary != nil {
		proj.count = summary.GetCount()
		proj.credentialRequired = summary.GetCount() != 0 &&
			(!hasEntityKeyStore || unlockedCount == 0)
		proj.skipped = skippedKey != "" && skippedKey == summary.GetGenerationKey()
	}
	if run != nil {
		proj.running = run.Running
		proj.failed = len(run.Failures) != 0
	}
	return proj
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
	for i := len(releases) - 1; i >= 0; i-- {
		if releases[i] != nil {
			releases[i]()
		}
	}
}
