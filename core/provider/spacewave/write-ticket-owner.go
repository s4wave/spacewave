package provider_spacewave

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/provider/spacewave/writeticketowner"
)

// writeTicketAudience identifies one bundled write-ticket capability.
type writeTicketAudience = writeticketowner.Audience

// write-ticket audience constants.
const (
	writeTicketAudienceSOOp           = writeticketowner.AudienceSOOp
	writeTicketAudienceSORoot         = writeticketowner.AudienceSORoot
	writeTicketAudienceBstoreSyncPush = writeticketowner.AudienceBstoreSyncPush
)

// newWriteTicketOwner constructs a write-ticket owner for one resource.
func newWriteTicketOwner(acc *ProviderAccount, resourceID string) *writeticketowner.Owner {
	return writeticketowner.NewOwner(
		func(ctx context.Context) (writeticketowner.Fetcher, error) {
			cli, _, _, err := acc.getReadySessionClient(ctx)
			if err != nil {
				return nil, err
			}
			return writeTicketSessionFetcher{cli: cli}, nil
		},
		resourceID,
		writeTicketBundleRefCountOptions,
		isRefreshableWriteTicketCloudError,
	)
}

type writeTicketSessionFetcher struct {
	cli *SessionClient
}

func (f writeTicketSessionFetcher) GetWriteTicketBundle(
	ctx context.Context,
	resourceID string,
) (*api.WriteTicketBundleResponse, error) {
	return f.cli.GetWriteTicketBundle(ctx, resourceID)
}

func (f writeTicketSessionFetcher) GetWriteTicket(
	ctx context.Context,
	resourceID string,
	audience writeticketowner.Audience,
) (string, error) {
	return f.cli.GetWriteTicket(ctx, resourceID, string(audience))
}

func validateWriteTicketAudience(audience writeTicketAudience) error {
	return writeticketowner.ValidateAudience(audience)
}

// setWriteTicketOwnersContext updates the lifecycle context for ticket owners.
func (a *ProviderAccount) setWriteTicketOwnersContext(ctx context.Context) {
	a.writeTicketOwnersMtx.Lock()
	a.writeTicketOwnersCtx = ctx
	owners := make([]*writeticketowner.Owner, 0, len(a.writeTicketOwners))
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
func (a *ProviderAccount) getWriteTicketOwner(resourceID string) *writeticketowner.Owner {
	a.writeTicketOwnersMtx.Lock()
	if a.writeTicketOwners == nil {
		a.writeTicketOwners = make(map[string]*writeticketowner.Owner)
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
