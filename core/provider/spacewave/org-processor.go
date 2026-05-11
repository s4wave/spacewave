package provider_spacewave

import (
	"context"
	"slices"

	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
)

// buildOrgProcessorRoutine returns the keyed routine for an org processor.
func (a *ProviderAccount) buildOrgProcessorRoutine(orgID string) (keyed.Routine, struct{}) {
	return func(ctx context.Context) error {
		ref := sobject.NewSharedObjectRef(a.GetProviderID(), a.accountID, orgID, SobjectBlockStoreID(orgID))

		so, soRef, err := sobject.ExMountSharedObject(ctx, a.p.b, ref, false, nil)
		if err != nil {
			if isTerminalSharedObjectMountError(err) {
				a.le.WithError(err).
					WithField("org-id", orgID).
					Warn("org processor hit terminal shared object mount error")
				return nil
			}
			return err
		}
		defer soRef.Release()

		return so.ProcessOperations(ctx, true, s4wave_org.ProcessOrgOps)
	}, struct{}{}
}

// watchOrgProcessors watches the SO list and starts/stops org processors
// as org-typed SOs appear or disappear. Blocks until ctx is canceled.
func (a *ProviderAccount) watchOrgProcessors(ctx context.Context) error {
	processors := keyed.NewKeyedWithLogger[string, struct{}](
		a.buildOrgProcessorRoutine,
		a.le.WithField("subsystem", "org-processors"),
		keyed.WithRetry[string, struct{}](&backoff.Backoff{
			BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
		}),
	)
	processors.SetContext(ctx, true)
	defer processors.SetContext(nil, false)

	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	soList, err := a.soListCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	soListChanged := make(chan *sobject.SharedObjectList, 1)
	orgListChanged := make(chan struct{}, 1)
	errCh := make(chan error, 2)

	go a.watchOrgProcessorSOList(watchCtx, soList, soListChanged, errCh)
	go a.watchOrgProcessorOrgList(watchCtx, orgListChanged)

	var prevOrgIDs []string
	for {
		orgIDs := a.orgProcessorKeys(soList)
		if !slices.Equal(orgIDs, prevOrgIDs) {
			processors.SyncKeys(orgIDs, false)
			prevOrgIDs = orgIDs
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		case next := <-soListChanged:
			soList = next
		case <-orgListChanged:
		}
	}
}

func (a *ProviderAccount) watchOrgProcessorSOList(
	ctx context.Context,
	current *sobject.SharedObjectList,
	changed chan<- *sobject.SharedObjectList,
	errCh chan<- error,
) {
	for {
		next, err := a.soListCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			select {
			case errCh <- err:
			case <-ctx.Done():
			}
			return
		}
		current = next
		select {
		case changed <- next:
		case <-ctx.Done():
			return
		}
	}
}

func (a *ProviderAccount) watchOrgProcessorOrgList(
	ctx context.Context,
	changed chan<- struct{},
) {
	for {
		var ch <-chan struct{}
		a.orgBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
		})
		select {
		case <-ctx.Done():
			return
		case <-ch:
		}
		select {
		case changed <- struct{}{}:
		default:
		}
	}
}

func (a *ProviderAccount) orgProcessorKeys(
	soList *sobject.SharedObjectList,
) []string {
	if soList == nil {
		return nil
	}

	ownerOrgs := make(map[string]struct{})
	a.orgBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !a.orgListValid {
			return
		}
		for _, org := range a.orgList {
			if isOrganizationOwnerRole(org.GetRole()) {
				ownerOrgs[org.GetId()] = struct{}{}
			}
		}
	})
	if len(ownerOrgs) == 0 {
		return nil
	}

	var orgIDs []string
	for _, entry := range soList.GetSharedObjects() {
		if entry.GetMeta().GetBodyType() != s4wave_org.OrgBodyType {
			continue
		}
		orgID := entry.GetRef().GetProviderResourceRef().GetId()
		if _, ok := ownerOrgs[orgID]; !ok {
			continue
		}
		orgIDs = append(orgIDs, orgID)
	}
	return orgIDs
}
