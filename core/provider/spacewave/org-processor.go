package provider_spacewave

import (
	"context"
	"slices"
	"sync"

	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/keyed"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/provider/spacewave/orgprocessor"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
)

type orgProcessorKeySet interface {
	SyncKeys(keys []string, restart bool) (added, removed []string)
}

type orgProcessorHooks struct {
	afterInitialSync func()
}

// buildOrgProcessorRoutine returns the keyed routine for an org processor.
func (a *ProviderAccount) buildOrgProcessorRoutine(orgID string) (keyed.Routine, struct{}) {
	return func(ctx context.Context) error {
		// Mount the organization shared object for processing.
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

		// Process organization operations until cancellation.
		return so.ProcessOperations(ctx, true, s4wave_org.ProcessOrgOps)
	}, struct{}{}
}

// watchOrgProcessors watches the SO list and starts/stops org processors
// as org-typed SOs appear or disappear. Blocks until ctx is canceled.
func (a *ProviderAccount) watchOrgProcessors(ctx context.Context) error {
	// Configure keyed retries for organization processors.
	processors := keyed.NewKeyedWithLogger[string, struct{}](
		a.buildOrgProcessorRoutine,
		a.le.WithField("subsystem", "org-processors"),
		keyed.WithRetry[string, struct{}](&backoff.Backoff{
			BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
		}),
	)
	processors.SetContext(ctx, true)
	defer processors.SetContext(nil, false)

	// Wait for the shared-object list before selecting processors.
	soList, err := a.soListCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	// Start processors for the initial organization set.
	desired := newOrgProcessorDesiredKeys(a, processors, soList)
	if err := a.runOrgProcessorDesiredKeys(ctx, desired); err != nil {
		return err
	}
	return nil
}

func (a *ProviderAccount) runOrgProcessorDesiredKeys(
	ctx context.Context,
	desired *orgProcessorDesiredKeys,
) error {
	return a.runOrgProcessorDesiredKeysWithHooks(ctx, desired, orgProcessorHooks{})
}

func (a *ProviderAccount) runOrgProcessorDesiredKeysWithHooks(
	ctx context.Context,
	desired *orgProcessorDesiredKeys,
	hooks orgProcessorHooks,
) error {
	// Apply the initial desired processor set.
	desired.sync()
	if hooks.afterInitialSync != nil {
		hooks.afterInitialSync()
	}

	// Watch shared-object changes and reconcile processor keys.
	soListWatcher := newNamedRoutineContainer(a.le, "org-processor-so-list")
	soListWatcher.SetRoutine(desired.watchSOList)
	soListWatcher.SetContext(ctx, true)
	defer soListWatcher.ClearContext()

	// Watch organization changes and reconcile processor keys.
	orgListWatcher := newNamedRoutineContainer(a.le, "org-processor-org-list")
	orgListWatcher.SetRoutine(desired.watchOrgList)
	orgListWatcher.SetContext(ctx, true)
	defer orgListWatcher.ClearContext()

	// Wait for a watcher to exit or context cancellation.
	return soListWatcher.WaitExited(ctx, false, nil)
}

type orgProcessorDesiredKeys struct {
	a          *ProviderAccount
	processors orgProcessorKeySet
	mtx        sync.Mutex
	soList     *sobject.SharedObjectList
	orgIDs     []string
}

func newOrgProcessorDesiredKeys(
	a *ProviderAccount,
	processors orgProcessorKeySet,
	soList *sobject.SharedObjectList,
) *orgProcessorDesiredKeys {
	return &orgProcessorDesiredKeys{
		a:          a,
		processors: processors,
		soList:     soList,
	}
}

func (d *orgProcessorDesiredKeys) watchSOList(ctx context.Context) error {
	current := d.currentSOList()
	for {
		next, err := d.a.soListCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			return err
		}
		current = next
		d.setSOList(next)
	}
}

func (d *orgProcessorDesiredKeys) watchOrgList(ctx context.Context) error {
	for {
		var ch <-chan struct{}
		d.a.orgBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
		})
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
		d.sync()
	}
}

func (d *orgProcessorDesiredKeys) currentSOList() *sobject.SharedObjectList {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return d.soList
}

func (d *orgProcessorDesiredKeys) setSOList(next *sobject.SharedObjectList) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.soList = next
	d.syncLocked()
}

func (d *orgProcessorDesiredKeys) sync() {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.syncLocked()
}

func (d *orgProcessorDesiredKeys) syncLocked() {
	orgIDs := d.a.orgProcessorKeys(d.soList)
	if slices.Equal(orgIDs, d.orgIDs) {
		return
	}
	d.processors.SyncKeys(orgIDs, false)
	d.orgIDs = orgIDs
}

func (a *ProviderAccount) orgProcessorKeys(
	soList *sobject.SharedObjectList,
) []string {
	if soList == nil {
		return nil
	}

	var orgs []*api.OrgResponse
	var valid bool
	a.orgBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		valid = a.orgListValid
		orgs = append(orgs, a.orgList...)
	})
	return orgprocessor.Keys(soList, orgs, valid)
}
