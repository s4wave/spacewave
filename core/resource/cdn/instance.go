package resource_cdn

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/cdn"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	cdn_sharedobject "github.com/s4wave/spacewave/core/cdn/sharedobject"
	"github.com/sirupsen/logrus"
)

// CdnInstance owns the process-scoped singleton CdnSharedObject and the
// shared CdnBlockStore backing it for a single CDN (identified by cdn_id in
// the enclosing registry). Future multi-CDN support will create one
// CdnInstance per configured CDN; today only the default instance exists.
//
// CdnInstance is process-lived: started once by the Registry on first
// lookup, torn down via Close when the Registry is stopped.
type CdnInstance struct {
	// spaceID is the CDN Space ULID.
	spaceID string
	// so is the read-only SharedObject for the CDN Space.
	so *cdn_sharedobject.CdnSharedObject
	// bs is the block store backing so.
	bs *cdn_bstore.CdnBlockStore
	// refresh runs bs.Invalidate + so.RefreshSnapshot in response to
	// RestartRoutine triggers from upstream notifications. Scoped to the
	// instance lifetime context.
	refresh *routine.RoutineContainer
}

// newCdnInstance builds a CdnInstance for the supplied CDN Space id.
// The instance installs a refresh routine scoped to ctx so subsequent
// root-changed notifications (wired in iter 4) can trigger a cheap
// bs.Invalidate + so.RefreshSnapshot without spawning goroutines.
func newCdnInstance(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	spaceID string,
) (*CdnInstance, error) {
	if spaceID == "" {
		return nil, errors.New("cdn space id is required")
	}
	bs, err := cdn_bstore.NewCdnBlockStore(cdn_bstore.Options{
		CdnBaseURL: cdn.BaseURL(),
		SpaceID:    spaceID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "build cdn block store")
	}
	so, err := cdn_sharedobject.NewCdnSharedObject(cdn_sharedobject.CdnSharedObjectOptions{
		SpaceID:    spaceID,
		Bus:        b,
		BlockStore: bs,
	})
	if err != nil {
		return nil, errors.Wrap(err, "build cdn shared object")
	}
	refresh := routine.NewRoutineContainerWithLogger(le)
	refresh.SetRoutine(func(rctx context.Context) error {
		bs.Invalidate()
		if refreshErr := so.RefreshSnapshot(rctx); refreshErr != nil {
			if rctx.Err() != nil {
				return rctx.Err()
			}
			le.WithError(refreshErr).
				WithField("space-id", spaceID).
				Warn("cdn instance: RefreshSnapshot failed")
		}
		return nil
	})
	refresh.SetContext(ctx, false)
	return &CdnInstance{
		spaceID: spaceID,
		so:      so,
		bs:      bs,
		refresh: refresh,
	}, nil
}

// GetSpaceID returns the CDN Space ULID.
func (c *CdnInstance) GetSpaceID() string {
	return c.spaceID
}

// GetSharedObject returns the underlying CDN SharedObject.
func (c *CdnInstance) GetSharedObject() *cdn_sharedobject.CdnSharedObject {
	return c.so
}

// GetBlockStore returns the CDN block store.
func (c *CdnInstance) GetBlockStore() *cdn_bstore.CdnBlockStore {
	return c.bs
}

// Refresh triggers a cheap invalidate + snapshot refresh. Called by upstream
// notification plumbing (cdn-root-changed WS frame, wired in iter 4) so
// mounted clients see a new snapshot shortly after the CDN root moves.
func (c *CdnInstance) Refresh() {
	c.refresh.RestartRoutine()
}

// Close stops the refresh routine. The underlying SharedObject and block
// store have no teardown of their own.
func (c *CdnInstance) Close() {
	if c.refresh != nil {
		c.refresh.ClearContext()
	}
}
