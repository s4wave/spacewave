package provider_spacewave

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/provider/spacewave/seedflight"
)

// targetedInvitationsCache caches the account's targeted invitation inbox.
// All fields are guarded by bcast, which fires only for inbox changes.
type targetedInvitationsCache struct {
	// bcast guards the fields below and wakes inbox watchers.
	bcast broadcast.Broadcast
	// resp is the last fetched inbox.
	resp *api.ListTargetedInvitationsResponse
	// valid indicates resp reflects every invalidation so far.
	valid bool
	// gen increments on every invalidation, so a fetch that raced one does
	// not mark its result valid.
	gen uint64
	// seed shares one inbox fetch across concurrent callers.
	seed seedflight.Seed
}

// GetTargetedInvitations returns the account's targeted invitation inbox and
// a channel closed when the inbox is next invalidated. The inbox is fetched
// once per invalidation, and concurrent callers share the fetch.
func (a *ProviderAccount) GetTargetedInvitations(
	ctx context.Context,
) (*api.ListTargetedInvitationsResponse, <-chan struct{}, error) {
	c := &a.targetedInvitations
	for {
		// Fetch the inbox unless the cached copy is current.
		if err := c.seed.RunWhenReady(
			ctx,
			&c.bcast,
			func() bool { return c.valid },
			a.fetchTargetedInvitations,
		); err != nil {
			return nil, nil, err
		}

		// Read the inbox with its wait channel, fetching again when an
		// invalidation landed after the fetch.
		var resp *api.ListTargetedInvitationsResponse
		var waitCh <-chan struct{}
		c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if c.valid {
				resp, waitCh = c.resp, getWaitCh()
			}
		})
		if waitCh != nil {
			return resp, waitCh, nil
		}
	}
}

// InvalidateTargetedInvitations marks the cached inbox stale and wakes its
// watchers, which fetch it again.
func (a *ProviderAccount) InvalidateTargetedInvitations() {
	c := &a.targetedInvitations
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.gen++
		c.valid = false
		broadcast()
	})
}

// fetchTargetedInvitations fetches the inbox from the cloud and stores it,
// valid only when no invalidation arrived during the fetch.
func (a *ProviderAccount) fetchTargetedInvitations(ctx context.Context) error {
	// Note the generation the fetch starts from.
	c := &a.targetedInvitations
	var gen uint64
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		gen = c.gen
	})

	// Fetch and store the inbox.
	// Resolve the current signer before the cloud request.
	cli, err := a.GetSessionClient()
	if err != nil {
		return err
	}

	// Fetch the invitation inbox before publishing its snapshot.
	resp, err := cli.ListTargetedInvitations(ctx)
	if err != nil {
		return err
	}
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		c.resp = resp
		c.valid = c.gen == gen
	})
	return nil
}
