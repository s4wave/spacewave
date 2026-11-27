package provider_spacewave

import (
	"context"
	"net/http"
	"path"

	api "github.com/aperturerobotics/spacewave-cloud/api"
	"github.com/aperturerobotics/util/refcount"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
	"github.com/s4wave/spacewave/core/sobject"
)

// sharedObjectLookupOptions honors a server retry deadline while demand remains.
// Other transport failures and refusals are returned for an explicit retry.
var sharedObjectLookupOptions = &refcount.Options{
	RetryBackoff: providerBackoff,
	ShouldRetry: func(err error) bool {
		return clouderror.RetryAfter(err) > 0 && !clouderror.IsNonRetryable(err)
	},
	RetryDelay: providerRetryDelay,
}

// LookupSObject reads one descriptor through the catalog-membership route.
func (c *SessionClient) LookupSObject(ctx context.Context, id string) (*sobject.SharedObjectListEntry, error) {
	// Validate the path argument before resolving the signed route.
	if _, err := ulid.ParseULID(id); err != nil {
		return nil, err
	}

	// Decode the binary response without treating resource refusals as absence.
	out := &api.LookupSObjectResponse{}
	data, err := c.doGetBinary(ctx, path.Join("/api/sobject", id, "lookup"), SeedReasonColdSeed)
	if err != nil {
		return nil, err
	}
	if err := out.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "decode shared object lookup")
	}
	if out.GetEntry() == nil {
		return nil, errors.New("lookup returned no descriptor")
	}
	return out.Entry, nil
}

// LookupSharedObject resolves one catalog miss without waiting for its projection.
// Concurrent demand shares the request; each caller releases only its own reference.
func (a *ProviderAccount) LookupSharedObject(ctx context.Context, id string) (*sobject.SharedObjectListEntry, error) {
	// Reuse the existing descriptor cache before attempting any cloud access.
	for _, entry := range a.soListCtr.GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == id {
			return entry, nil
		}
	}
	if id == "" {
		return nil, errors.New("shared object id is required")
	}

	// Attach demand to this account's existing metadata owner.
	var lookup *refcount.RefCount[*sobject.SharedObjectListEntry]
	var deleted bool
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		state := a.getOrCreateSharedObjectMetadataStateLocked(id)
		deleted = state.status == sharedObjectMetadataDeleted
		if state.lookup == nil {
			state.lookup = refcount.NewRefCountWithOptions(
				context.Background(), false, nil, nil,
				func(ctx context.Context, _ func()) (*sobject.SharedObjectListEntry, func(), error) {
					entry, err := a.lookupSharedObject(ctx, id)
					return entry, nil, err
				},
				sharedObjectLookupOptions,
			)
		}
		lookup = state.lookup
	})
	if deleted {
		return nil, ErrSharedObjectMetadataDeleted
	}

	// The last demand releases and cancels the shared request.
	entry, release, err := lookup.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return entry, nil
}

// lookupSharedObject fetches under one signer, connection and cache generation.
func (a *ProviderAccount) lookupSharedObject(ctx context.Context, id string) (*sobject.SharedObjectListEntry, error) {
	// Capture all authority boundaries together before issuing the request.
	var cli *SessionClient
	var gen, cacheGen uint64
	var unsupported bool
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		cli = a.sessionClient
		gen = a.soLookupGeneration
		cacheGen = a.getOrCreateSharedObjectMetadataStateLocked(id).gen
		unsupported = cli != nil && a.soLookupUnsupported == cli
	})
	if unsupported {
		return nil, sobject.ErrSharedObjectLookupUnsupported
	}
	if cli == nil || !cli.GetSessionSigner().CanSign() {
		return nil, ErrSigningUnavailable
	}

	// Discover route support lazily with the operation the caller needs.
	entry, err := cli.LookupSObject(ctx, id)
	var ce *cloudError
	absent := errors.As(err, &ce) && ce.StatusCode == http.StatusNotFound && ce.Code == "not_found"
	if err == nil {
		prr := entry.GetRef().GetProviderResourceRef()
		if prr != nil && prr.GetProviderId() == "" {
			prr.ProviderId = a.p.info.GetProviderId()
		}
		if err = entry.GetRef().Validate(); err == nil &&
			(prr.GetId() != id || prr.GetProviderAccountId() != a.accountID || prr.GetProviderId() != a.p.info.GetProviderId()) {
			err = errors.New("lookup descriptor does not match the requested account and object")
		}
	}

	// Admit both the support result and descriptor only in their original owner.
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Reject replies whose demand or authority ended before publication.
		state := a.getOrCreateSharedObjectMetadataStateLocked(id)
		if ctx.Err() != nil {
			err = ctx.Err()
			return
		}
		if cli != a.sessionClient || gen != a.soLookupGeneration || state.gen != cacheGen ||
			state.status == sharedObjectMetadataDeleted || !cli.GetSessionSigner().CanSign() ||
			a.state.status == provider.ProviderAccountStatus_ProviderAccountStatus_DELETED {
			err = errors.New("shared object lookup was invalidated")
			return
		}

		// Remember only the old dispatcher in this live connection generation.
		if absent {
			a.soLookupUnsupported = cli
			err = sobject.ErrSharedObjectLookupUnsupported
		}
		if err == nil {
			a.cacheSharedObjectListEntry(entry)
		}
	})
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// invalidateSharedObjectLookup rejects pending replies after an access change.
func (a *ProviderAccount) invalidateSharedObjectLookup() {
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		a.soLookupGeneration++
	})
}

// resetSharedObjectLookup discovers support again after an authenticated reconnect.
func (a *ProviderAccount) resetSharedObjectLookup() {
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		a.soLookupGeneration++
		a.soLookupUnsupported = nil
	})
}
