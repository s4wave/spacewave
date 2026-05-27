package writeticketowner

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/promise"
	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// Audience identifies one bundled write-ticket capability.
type Audience string

// Write-ticket audiences.
const (
	AudienceSOOp           Audience = "so-op"
	AudienceSORoot         Audience = "so-root"
	AudienceBstoreSyncPush Audience = "bstore-sync-push"
)

// Fetcher fetches write-ticket bundles and individual audience tickets.
type Fetcher interface {
	GetWriteTicketBundle(ctx context.Context, resourceID string) (*api.WriteTicketBundleResponse, error)
	GetWriteTicket(ctx context.Context, resourceID string, audience Audience) (string, error)
}

// FetcherProvider returns the session-scoped fetcher used for one cloud fetch.
type FetcherProvider func(ctx context.Context) (Fetcher, error)

// RefreshableError reports whether an execution error means a ticket should be
// refreshed and retried once.
type RefreshableError func(error) bool

// Owner caches bundled write tickets for one cloud resource.
type Owner struct {
	fetcherProvider FetcherProvider
	resourceID      string
	refreshableErr  RefreshableError

	bcast  broadcast.Broadcast
	bundle *api.WriteTicketBundleResponse
	// invalidate restarts the owner when the current bundle becomes stale.
	invalidate func()
	// audienceRefresh tracks one in-flight targeted refresh per audience.
	audienceRefresh map[Audience]*promise.Promise[string]

	rc *refcount.RefCount[struct{}]
}

// NewOwner constructs a write-ticket owner.
func NewOwner(
	fetcherProvider FetcherProvider,
	resourceID string,
	opts *refcount.Options,
	refreshableErr RefreshableError,
) *Owner {
	o := &Owner{
		fetcherProvider: fetcherProvider,
		resourceID:      resourceID,
		refreshableErr:  refreshableErr,
	}
	o.rc = refcount.NewRefCountWithOptions(
		context.Background(),
		true,
		nil,
		nil,
		o.resolve,
		opts,
	)
	return o
}

// SetContext sets the owner lifecycle context.
func (o *Owner) SetContext(ctx context.Context) {
	_ = o.rc.SetContext(ctx)
}

// ClearContext clears the owner lifecycle context.
func (o *Owner) ClearContext() {
	o.rc.ClearContext()
}

// Resolve resolves the bundled write-ticket snapshot for this resource.
func (o *Owner) Resolve(
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
func (o *Owner) ExecuteAudience(
	ctx context.Context,
	audience Audience,
	fn func(ticket string) error,
) error {
	if err := ValidateAudience(audience); err != nil {
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
	if o.refreshableErr == nil || !o.refreshableErr(err) {
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
func (o *Owner) RefreshAudience(
	ctx context.Context,
	audience Audience,
) (string, error) {
	if err := ValidateAudience(audience); err != nil {
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

	fetcher, err := o.fetcher(ctx)
	if err != nil {
		o.finishAudienceRefresh(audience, prom, "", err)
		return prom.Await(ctx)
	}
	ticket, err := fetcher.GetWriteTicket(ctx, o.resourceID, audience)
	o.finishAudienceRefresh(audience, prom, ticket, err)
	return prom.Await(ctx)
}

// Invalidate restarts the owner when the current bundle should be discarded.
func (o *Owner) Invalidate() {
	var invalidate func()
	o.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		invalidate = o.invalidate
	})
	if invalidate != nil {
		invalidate()
	}
}

// InvalidateAudience clears one cached audience while preserving the others.
func (o *Owner) InvalidateAudience(audience Audience) error {
	if err := ValidateAudience(audience); err != nil {
		return err
	}

	o.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if o.bundle == nil {
			return
		}
		if GetBundleAudience(o.bundle, audience) == "" {
			return
		}
		SetBundleAudience(o.bundle, audience, "")
		broadcast()
	})
	return nil
}

func (o *Owner) getBundle() *api.WriteTicketBundleResponse {
	var bundle *api.WriteTicketBundleResponse
	o.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if o.bundle != nil {
			bundle = o.bundle.CloneVT()
		}
	})
	return bundle
}

func (o *Owner) getAudienceTicket(
	ctx context.Context,
	audience Audience,
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

	ticket := GetBundleAudience(bundle, audience)
	if ticket != "" {
		return ticket, nil
	}
	return o.RefreshAudience(ctx, audience)
}

func (o *Owner) startAudienceRefresh(
	audience Audience,
) (*promise.Promise[string], bool) {
	var prom *promise.Promise[string]
	var owner bool
	o.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if o.audienceRefresh == nil {
			o.audienceRefresh = make(map[Audience]*promise.Promise[string])
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

func (o *Owner) finishAudienceRefresh(
	audience Audience,
	prom *promise.Promise[string],
	ticket string,
	err error,
) {
	var shouldBroadcast bool
	o.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if err == nil && o.bundle != nil {
			if GetBundleAudience(o.bundle, audience) != ticket {
				SetBundleAudience(o.bundle, audience, ticket)
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

func (o *Owner) resolve(
	ctx context.Context,
	released func(),
) (struct{}, func(), error) {
	fetcher, err := o.fetcher(ctx)
	if err != nil {
		return struct{}{}, nil, err
	}

	bundle, err := fetcher.GetWriteTicketBundle(ctx, o.resourceID)
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

func (o *Owner) fetcher(ctx context.Context) (Fetcher, error) {
	if o.fetcherProvider == nil {
		return nil, errors.New("session client not ready")
	}
	fetcher, err := o.fetcherProvider(ctx)
	if err != nil {
		return nil, err
	}
	if fetcher == nil {
		return nil, errors.New("session client not ready")
	}
	return fetcher, nil
}

// ValidateAudience validates a write-ticket audience.
func ValidateAudience(audience Audience) error {
	switch audience {
	case AudienceSOOp, AudienceSORoot, AudienceBstoreSyncPush:
		return nil
	default:
		return errors.Errorf("unknown write ticket audience: %s", audience)
	}
}

// GetBundleAudience returns the ticket for one audience in a bundled response.
func GetBundleAudience(
	bundle *api.WriteTicketBundleResponse,
	audience Audience,
) string {
	if bundle == nil {
		return ""
	}

	switch audience {
	case AudienceSOOp:
		return bundle.GetSoOpTicket()
	case AudienceSORoot:
		return bundle.GetSoRootTicket()
	case AudienceBstoreSyncPush:
		return bundle.GetBstoreSyncPushTicket()
	default:
		return ""
	}
}

// SetBundleAudience sets the ticket for one audience in a bundled response.
func SetBundleAudience(
	bundle *api.WriteTicketBundleResponse,
	audience Audience,
	ticket string,
) {
	if bundle == nil {
		return
	}

	switch audience {
	case AudienceSOOp:
		bundle.SoOpTicket = ticket
	case AudienceSORoot:
		bundle.SoRootTicket = ticket
	case AudienceBstoreSyncPush:
		bundle.BstoreSyncPushTicket = ticket
	}
}
