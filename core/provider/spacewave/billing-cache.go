package provider_spacewave

import (
	"context"

	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

type billingSnapshot struct {
	state *api.BillingStateResponse
	usage *api.BillingUsageResponse
}

// getBillingSnapshotRcLocked returns the billing snapshot cache for the given account ID.
func (a *ProviderAccount) getBillingSnapshotRcLocked(
	baID string,
) *refcount.RefCount[*billingSnapshot] {
	if baID == "" {
		return nil
	}
	if a.state.billingSnapshotRcs == nil {
		a.state.billingSnapshotRcs = make(map[string]*refcount.RefCount[*billingSnapshot])
	}
	return getOrCreateSnapshotRefCount(
		&a.state.billingSnapshotRcs,
		baID,
		func(ctx context.Context, key string, released func()) (*billingSnapshot, func(), error) {
			return a.resolveBillingSnapshot(ctx, key, released)
		},
	)
}

// InvalidateBillingSnapshot invalidates a cached billing snapshot.
func (a *ProviderAccount) InvalidateBillingSnapshot(baID string) {
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if baID == "" {
			for _, rc := range a.state.billingSnapshotRcs {
				rc.Invalidate()
			}
			broadcast()
			return
		}
		rc := a.getBillingSnapshotRcLocked(baID)
		if rc != nil {
			rc.Invalidate()
		}
		broadcast()
	})
}

// GetBillingSnapshot returns cached billing state and usage, fetching on cache miss.
func (a *ProviderAccount) GetBillingSnapshot(
	ctx context.Context,
	baID string,
) (*api.BillingStateResponse, *api.BillingUsageResponse, error) {
	if baID == "" {
		return nil, nil, errors.New("billing account id is required")
	}

	var rc *refcount.RefCount[*billingSnapshot]
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		rc = a.getBillingSnapshotRcLocked(baID)
	})
	snapshot, rel, err := rc.Resolve(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer rel()
	if snapshot == nil {
		return nil, nil, errors.New("billing snapshot not available")
	}
	var state *api.BillingStateResponse
	if snapshot.state != nil {
		state = snapshot.state.CloneVT()
	}
	var usage *api.BillingUsageResponse
	if snapshot.usage != nil {
		usage = snapshot.usage.CloneVT()
	}
	return state, usage, nil
}

// resolveBillingSnapshot fetches billing state and usage from the cloud.
func (a *ProviderAccount) resolveBillingSnapshot(
	ctx context.Context,
	baID string,
	_ func(),
) (*billingSnapshot, func(), error) {
	cli := a.GetSessionClient()
	if cli == nil {
		return nil, nil, errors.New("session client not available")
	}

	stateData, err := cli.GetBillingState(ctx, baID)
	if err != nil {
		return nil, nil, err
	}
	state := &api.BillingStateResponse{}
	if err := state.UnmarshalVT(stateData); err != nil {
		return nil, nil, errors.Wrap(err, "unmarshal billing state")
	}

	usageData, err := cli.GetBillingUsage(ctx, baID)
	if err != nil {
		return nil, nil, err
	}
	usage := &api.BillingUsageResponse{}
	if err := usage.UnmarshalVT(usageData); err != nil {
		return nil, nil, errors.Wrap(err, "unmarshal billing usage")
	}

	return &billingSnapshot{
		state: state,
		usage: usage,
	}, nil, nil
}
