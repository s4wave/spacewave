package provider_spacewave

import (
	"context"
	"net/http"
	"path"

	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/refcount"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
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

// sharedObjectLookups holds the account's catalog-miss lookups.
//
// An admitted descriptor lets the mount path open a Space the catalog has not
// listed yet. It is kept apart from soListCtr, so a lookup never publishes or
// stands in for the catalog, and a stale catalog snapshot cannot drop it. Every
// generation bump clears the admitted descriptors, so none outlives an access
// change, catalog push, sign-out or reconnect the client has seen.
type sharedObjectLookups struct {
	// generation increments on reconnects and access changes.
	generation uint64
	// unsupported is the client whose current connection lacks the route.
	unsupported *SessionClient
	// demand holds each id's shared request while a caller waits for it.
	demand map[string]*sharedObjectLookupDemand
	// admitted holds the descriptors admitted under generation, by id.
	admitted map[string]*sobject.SharedObjectListEntry
}

// sharedObjectLookupDemand shares one id's request among its waiting callers.
// The last caller to leave removes it, so a miss leaves no state behind.
type sharedObjectLookupDemand struct {
	// rc runs the request while any caller holds a reference.
	rc *refcount.RefCount[*sobject.SharedObjectListEntry]
	// callers counts the callers attached to rc.
	callers int
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
	// Serve a catalog hit before attempting any cloud access.
	for _, entry := range a.soListCtr.GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == id {
			return entry, nil
		}
	}
	if id == "" {
		return nil, errors.New("shared object id is required")
	}

	// Serve an admitted descriptor, or attach to the id's shared request.
	var entry *sobject.SharedObjectListEntry
	var demand *sharedObjectLookupDemand
	var deleted bool
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// A known deletion and an admitted descriptor need no request.
		if state := a.state.sharedObjectMetadata[id]; state != nil && state.status == sharedObjectMetadataDeleted {
			deleted = true
			return
		}
		if entry = a.soLookup.admitted[id]; entry != nil {
			return
		}

		// Join the id's request, or start it for the first caller.
		demand = a.soLookup.demand[id]
		if demand == nil {
			demand = &sharedObjectLookupDemand{rc: refcount.NewRefCountWithOptions(
				context.Background(), false, nil, nil,
				func(ctx context.Context, _ func()) (*sobject.SharedObjectListEntry, func(), error) {
					entry, err := a.lookupSharedObject(ctx, id)
					return entry, nil, err
				},
				sharedObjectLookupOptions,
			)}
			if a.soLookup.demand == nil {
				a.soLookup.demand = make(map[string]*sharedObjectLookupDemand)
			}
			a.soLookup.demand[id] = demand
		}
		demand.callers++
	})
	if deleted {
		return nil, ErrSharedObjectMetadataDeleted
	}
	if entry != nil {
		return entry, nil
	}
	defer a.leaveSharedObjectLookup(id, demand)

	// The last caller's release cancels the shared request.
	entry, release, err := demand.rc.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	release()
	return entry, nil
}

// leaveSharedObjectLookup detaches one caller and drops the demand after the last.
func (a *ProviderAccount) leaveSharedObjectLookup(id string, demand *sharedObjectLookupDemand) {
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Remove the demand only if a newer one has not replaced it.
		demand.callers--
		if demand.callers == 0 && a.soLookup.demand[id] == demand {
			delete(a.soLookup.demand, id)
		}
	})
}

// lookupSharedObject fetches until a reply is admitted under the generation
// that is current when it arrives. A reply superseded by an invalidation or
// reconnect is discarded and the request repeats, so only a refusal, a
// transport failure or the end of ctx fails the caller.
//
// The first repeat runs at once, because the change that superseded the reply
// has already happened. Later repeats in the same call wait out the provider
// backoff, 0.5s growing to 1.8s, so a stream of invalidations costs a request
// per backoff step instead of one per round trip. ctx is the only limit.
func (a *ProviderAccount) lookupSharedObject(ctx context.Context, id string) (*sobject.SharedObjectListEntry, error) {
	// Each pass captures the generation current when it starts.
	var bo cbackoff.BackOff
	for {
		entry, superseded, err := a.lookupSharedObjectOnce(ctx, id)
		if !superseded {
			return entry, err
		}
		if bo == nil {
			bo = providerBackoff.Construct()
			continue
		}
		if err := waitProviderRetryDelay(ctx, nil, bo.NextBackOff()); err != nil {
			return nil, err
		}
	}
}

// lookupSharedObjectOnce fetches under one signer, connection and cache
// generation, and reports whether a newer one superseded the reply.
func (a *ProviderAccount) lookupSharedObjectOnce(ctx context.Context, id string) (*sobject.SharedObjectListEntry, bool, error) {
	// Capture all authority boundaries together before issuing the request.
	var cli *SessionClient
	var gen, cacheGen uint64
	var unsupported bool
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Read every boundary under one hold so they describe one moment.
		cli = a.sessionClient
		gen = a.soLookup.generation
		cacheGen = a.state.sharedObjectMetadata[id].getGen()
		unsupported = cli != nil && a.soLookup.unsupported == cli
	})
	if unsupported {
		return nil, false, sobject.ErrSharedObjectLookupUnsupported
	}
	if cli == nil || !cli.GetSessionSigner().CanSign() {
		return nil, false, ErrSigningUnavailable
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

	// Admit the support result and descriptor only under their own generation.
	var superseded bool
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// The caller's end and a known deletion are final.
		state := a.state.sharedObjectMetadata[id]
		switch {
		case ctx.Err() != nil:
			err = ctx.Err()
			return
		case state != nil && state.status == sharedObjectMetadataDeleted:
			err = ErrSharedObjectMetadataDeleted
			return
		case a.state.status == provider.ProviderAccountStatus_ProviderAccountStatus_DELETED:
			err = ErrProviderAccountDeleted
			return
		}

		// A newer signer, connection or access generation repeats the request.
		if cli != a.sessionClient || gen != a.soLookup.generation || state.getGen() != cacheGen {
			superseded = true
			return
		}

		// Remember only the old dispatcher in this live connection generation.
		if absent {
			a.soLookup.unsupported = cli
			err = sobject.ErrSharedObjectLookupUnsupported
		}
		if err == nil {
			if a.soLookup.admitted == nil {
				a.soLookup.admitted = make(map[string]*sobject.SharedObjectListEntry)
			}
			a.soLookup.admitted[id] = entry
		}
	})
	if superseded {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return entry, false, nil
}

// invalidateSharedObjectLookup supersedes pending replies and drops admitted
// descriptors after an access change.
func (a *ProviderAccount) invalidateSharedObjectLookup() {
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Supersede pending replies and forget admitted descriptors.
		a.soLookup.generation++
		a.soLookup.admitted = nil
	})
}

// resetSharedObjectLookup also discovers route support again after an
// authenticated reconnect.
func (a *ProviderAccount) resetSharedObjectLookup() {
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Supersede replies, forget descriptors and probe the route again.
		a.soLookup.generation++
		a.soLookup.admitted = nil
		a.soLookup.unsupported = nil
	})
}
