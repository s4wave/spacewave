package provider_spacewave

import (
	"context"
	"strings"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/promise"
	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// writeTicketAudience identifies one bundled write-ticket capability.
type writeTicketAudience string

// write-ticket audience constants.
const (
	writeTicketAudienceSOOp           writeTicketAudience = "so-op"
	writeTicketAudienceSORoot         writeTicketAudience = "so-root"
	writeTicketAudienceBstoreSyncPush writeTicketAudience = "bstore-sync-push"
)

// writeTicketOwner caches bundled write tickets for one cloud resource.
type writeTicketOwner struct {
	// acc owns the session client used to mint tickets.
	acc *ProviderAccount
	// resourceID is the shared resource identifier for all bundled audiences.
	resourceID string

	// bcast guards bundle and invalidate.
	bcast broadcast.Broadcast
	// bundle is the cached bundled write-ticket response.
	bundle *api.WriteTicketBundleResponse
	// invalidate restarts the owner when the current bundle becomes stale.
	invalidate func()
	// audienceRefresh tracks one in-flight targeted refresh per audience.
	audienceRefresh map[writeTicketAudience]*promise.Promise[string]

	// rc manages shared bundle acquisition.
	rc *refcount.RefCount[struct{}]
}

// newWriteTicketOwner constructs a writeTicketOwner.
func newWriteTicketOwner(acc *ProviderAccount, resourceID string) *writeTicketOwner {
	o := &writeTicketOwner{
		acc:        acc,
		resourceID: resourceID,
	}
	o.rc = refcount.NewRefCountWithOptions(
		nil,
		true,
		nil,
		nil,
		o.resolve,
		writeTicketBundleRefCountOptions,
	)
	return o
}

// SetContext sets the owner lifecycle context.
func (o *writeTicketOwner) SetContext(ctx context.Context) {
	_ = o.rc.SetContext(ctx)
}

// ClearContext clears the owner lifecycle context.
func (o *writeTicketOwner) ClearContext() {
	o.rc.ClearContext()
}

// Resolve resolves the bundled write-ticket snapshot for this resource.
func (o *writeTicketOwner) Resolve(
	ctx context.Context,
) (*api.WriteTicketBundleResponse, func(), error) {
	_, release, err := o.rc.Resolve(ctx)
	if err != nil {
		return nil, nil, err
	}

	bundle := o.getBundle()
	if bundle == nil {
		release()
		return nil, nil, errors.New("write ticket bundle missing after resolve")
	}
	return bundle, release, nil
}

// ExecuteAudience executes fn with the cached ticket for one audience and
// retries once after targeted refresh when the first attempt fails with an
// explicit stale or expired write-ticket error.
func (o *writeTicketOwner) ExecuteAudience(
	ctx context.Context,
	audience writeTicketAudience,
	fn func(ticket string) error,
) error {
	if err := validateWriteTicketAudience(audience); err != nil {
		return err
	}
	if fn == nil {
		return errors.New("missing write ticket callback")
	}

	ticket, err := o.getAudienceTicket(ctx, audience)
	if err != nil {
		return err
	}
	err = fn(ticket)
	if !isRefreshableWriteTicketCloudError(err) {
		return err
	}

	if err := o.InvalidateAudience(audience); err != nil {
		return err
	}
	ticket, err = o.RefreshAudience(ctx, audience)
	if err != nil {
		return err
	}
	return fn(ticket)
}

// RefreshAudience refreshes one write-ticket audience without tearing down the
// other cached capabilities.
func (o *writeTicketOwner) RefreshAudience(
	ctx context.Context,
	audience writeTicketAudience,
) (string, error) {
	if err := validateWriteTicketAudience(audience); err != nil {
		return "", err
	}

	if o.getBundle() == nil {
		_, release, err := o.Resolve(ctx)
		if err != nil {
			return "", err
		}
		release()
	}

	prom, owner := o.startAudienceRefresh(audience)
	if !owner {
		return prom.Await(ctx)
	}

	cli, _, _, err := o.acc.getReadySessionClient(ctx)
	if err != nil {
		o.finishAudienceRefresh(audience, prom, "", err)
		return prom.Await(ctx)
	}
	ticket, err := cli.GetWriteTicket(ctx, o.resourceID, string(audience))
	o.finishAudienceRefresh(audience, prom, ticket, err)
	return prom.Await(ctx)
}

// Invalidate restarts the owner when the current bundle should be discarded.
func (o *writeTicketOwner) Invalidate() {
	var invalidate func()
	o.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		invalidate = o.invalidate
	})
	if invalidate != nil {
		invalidate()
	}
}

// InvalidateAudience clears one cached audience while preserving the others.
func (o *writeTicketOwner) InvalidateAudience(audience writeTicketAudience) error {
	if err := validateWriteTicketAudience(audience); err != nil {
		return err
	}

	o.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if o.bundle == nil {
			return
		}
		if getWriteTicketBundleAudience(o.bundle, audience) == "" {
			return
		}
		setWriteTicketBundleAudience(o.bundle, audience, "")
		broadcast()
	})
	return nil
}

// getBundle returns a cloned bundled ticket snapshot.
func (o *writeTicketOwner) getBundle() *api.WriteTicketBundleResponse {
	var bundle *api.WriteTicketBundleResponse
	o.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if o.bundle != nil {
			bundle = o.bundle.CloneVT()
		}
	})
	return bundle
}

func (o *writeTicketOwner) getAudienceTicket(
	ctx context.Context,
	audience writeTicketAudience,
) (string, error) {
	bundle := o.getBundle()
	if bundle == nil {
		_, release, err := o.Resolve(ctx)
		if err != nil {
			return "", err
		}
		release()
		bundle = o.getBundle()
	}

	ticket := getWriteTicketBundleAudience(bundle, audience)
	if ticket != "" {
		return ticket, nil
	}
	return o.RefreshAudience(ctx, audience)
}

func (o *writeTicketOwner) startAudienceRefresh(
	audience writeTicketAudience,
) (*promise.Promise[string], bool) {
	var prom *promise.Promise[string]
	var owner bool
	o.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if o.audienceRefresh == nil {
			o.audienceRefresh = make(map[writeTicketAudience]*promise.Promise[string])
		}
		prom = o.audienceRefresh[audience]
		if prom != nil {
			return
		}
		prom = promise.NewPromise[string]()
		o.audienceRefresh[audience] = prom
		owner = true
	})
	return prom, owner
}

func (o *writeTicketOwner) finishAudienceRefresh(
	audience writeTicketAudience,
	prom *promise.Promise[string],
	ticket string,
	err error,
) {
	var shouldBroadcast bool
	o.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if err == nil && o.bundle != nil {
			if getWriteTicketBundleAudience(o.bundle, audience) != ticket {
				setWriteTicketBundleAudience(o.bundle, audience, ticket)
				shouldBroadcast = true
			}
		}
		if shouldBroadcast {
			broadcast()
		}
	})
	prom.SetResult(ticket, err)
	o.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if o.audienceRefresh[audience] == prom {
			delete(o.audienceRefresh, audience)
		}
	})
}

// resolve fetches the bundled write tickets once for the current references.
func (o *writeTicketOwner) resolve(
	ctx context.Context,
	released func(),
) (struct{}, func(), error) {
	cli, _, _, err := o.acc.getReadySessionClient(ctx)
	if err != nil {
		return struct{}{}, nil, err
	}

	bundle, err := cli.GetWriteTicketBundle(ctx, o.resourceID)
	if err != nil {
		return struct{}{}, nil, err
	}
	if bundle == nil {
		return struct{}{}, nil, errors.New("cloud returned nil write ticket bundle")
	}

	o.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		o.bundle = bundle.CloneVT()
		o.invalidate = released
		broadcast()
	})
	return struct{}{}, nil, nil
}

// setWriteTicketOwnersContext updates the lifecycle context for ticket owners.
func (a *ProviderAccount) setWriteTicketOwnersContext(ctx context.Context) {
	a.writeTicketOwnersMtx.Lock()
	a.writeTicketOwnersCtx = ctx
	owners := make([]*writeTicketOwner, 0, len(a.writeTicketOwners))
	for _, owner := range a.writeTicketOwners {
		owners = append(owners, owner)
	}
	a.writeTicketOwnersMtx.Unlock()

	for _, owner := range owners {
		if ctx == nil {
			owner.ClearContext()
			continue
		}
		owner.SetContext(ctx)
	}
}

// getWriteTicketOwner returns the bundled write-ticket owner for a resource.
func (a *ProviderAccount) getWriteTicketOwner(resourceID string) *writeTicketOwner {
	a.writeTicketOwnersMtx.Lock()
	if a.writeTicketOwners == nil {
		a.writeTicketOwners = make(map[string]*writeTicketOwner)
	}
	owner := a.writeTicketOwners[resourceID]
	if owner == nil {
		owner = newWriteTicketOwner(a, resourceID)
		a.writeTicketOwners[resourceID] = owner
	}
	ctx := a.writeTicketOwnersCtx
	a.writeTicketOwnersMtx.Unlock()

	if ctx != nil {
		owner.SetContext(ctx)
	}
	return owner
}

// GetWriteTicketBundle resolves the bundled write tickets for a resource.
func (a *ProviderAccount) GetWriteTicketBundle(
	ctx context.Context,
	resourceID string,
) (*api.WriteTicketBundleResponse, func(), error) {
	if resourceID == "" {
		return nil, nil, errors.New("missing resource id")
	}
	return a.getWriteTicketOwner(resourceID).Resolve(ctx)
}

// RefreshWriteTicketAudience refreshes one cached ticket audience for a resource.
func (a *ProviderAccount) RefreshWriteTicketAudience(
	ctx context.Context,
	resourceID string,
	audience writeTicketAudience,
) (string, error) {
	if strings.TrimSpace(resourceID) == "" {
		return "", errors.New("missing resource id")
	}
	return a.getWriteTicketOwner(resourceID).RefreshAudience(ctx, audience)
}

// InvalidateWriteTicketAudience clears one cached ticket audience for a resource.
func (a *ProviderAccount) InvalidateWriteTicketAudience(
	resourceID string,
	audience writeTicketAudience,
) error {
	if strings.TrimSpace(resourceID) == "" {
		return errors.New("missing resource id")
	}
	return a.getWriteTicketOwner(resourceID).InvalidateAudience(audience)
}

// ExecuteWriteTicketAudience executes fn with one audience ticket and retries
// once after targeted refresh on explicit refreshable ticket failures.
func (a *ProviderAccount) ExecuteWriteTicketAudience(
	ctx context.Context,
	resourceID string,
	audience writeTicketAudience,
	fn func(ticket string) error,
) error {
	if strings.TrimSpace(resourceID) == "" {
		return errors.New("missing resource id")
	}
	return a.getWriteTicketOwner(resourceID).ExecuteAudience(ctx, audience, fn)
}

func validateWriteTicketAudience(audience writeTicketAudience) error {
	switch audience {
	case writeTicketAudienceSOOp, writeTicketAudienceSORoot, writeTicketAudienceBstoreSyncPush:
		return nil
	default:
		return errors.Errorf("unknown write ticket audience: %s", audience)
	}
}

func getWriteTicketBundleAudience(
	bundle *api.WriteTicketBundleResponse,
	audience writeTicketAudience,
) string {
	if bundle == nil {
		return ""
	}

	switch audience {
	case writeTicketAudienceSOOp:
		return bundle.GetSoOpTicket()
	case writeTicketAudienceSORoot:
		return bundle.GetSoRootTicket()
	case writeTicketAudienceBstoreSyncPush:
		return bundle.GetBstoreSyncPushTicket()
	default:
		return ""
	}
}

func setWriteTicketBundleAudience(
	bundle *api.WriteTicketBundleResponse,
	audience writeTicketAudience,
	ticket string,
) {
	if bundle == nil {
		return
	}

	switch audience {
	case writeTicketAudienceSOOp:
		bundle.SoOpTicket = ticket
	case writeTicketAudienceSORoot:
		bundle.SoRootTicket = ticket
	case writeTicketAudienceBstoreSyncPush:
		bundle.BstoreSyncPushTicket = ticket
	}
}
