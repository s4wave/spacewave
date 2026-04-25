package provider_local

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
		providerID := a.t.accountInfo.GetProviderId()
		accountID := a.t.accountInfo.GetProviderAccountId()
		ref := sobject.NewSharedObjectRef(providerID, accountID, orgID, SobjectBlockStoreID(orgID))

		so, soRef, err := sobject.ExMountSharedObject(ctx, a.t.p.b, ref, false, nil)
		if err != nil {
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

	soList, err := a.soListCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	var prevOrgIDs []string
	for {
		var orgIDs []string
		for _, entry := range soList.GetSharedObjects() {
			if entry.GetMeta().GetBodyType() != s4wave_org.OrgBodyType {
				continue
			}
			orgIDs = append(orgIDs, entry.GetRef().GetProviderResourceRef().GetId())
		}
		if !slices.Equal(orgIDs, prevOrgIDs) {
			processors.SyncKeys(orgIDs, false)
			prevOrgIDs = orgIDs
		}

		soList, err = a.soListCtr.WaitValueChange(ctx, soList, nil)
		if err != nil {
			return err
		}
	}
}
