package cdn_bstore

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/cdn"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
)

// GrantSource issues grants for private CDN Spaces. A reader of a private
// Space asks it for a fresh grant before the current one expires.
type GrantSource interface {
	// SpaceGrant returns a fresh grant for spaceID.
	SpaceGrant(ctx context.Context, spaceID string) (*cdn.CdnSpaceGrant, error)
}

// GrantSourceFunc is a GrantSource as a function.
type GrantSourceFunc func(ctx context.Context, spaceID string) (*cdn.CdnSpaceGrant, error)

// SpaceGrant calls f.
func (f GrantSourceFunc) SpaceGrant(ctx context.Context, spaceID string) (*cdn.CdnSpaceGrant, error) {
	return f(ctx, spaceID)
}

// grantRetry is how long a reader keeps a grant that is still valid after a
// renewal failed before it asks again.
const grantRetry = 5 * time.Second

// ValidateGrant checks that g covers spaceID and is still valid at now.
func ValidateGrant(g *cdn.CdnSpaceGrant, spaceID string, now time.Time) error {
	if g.GetSpaceId() != spaceID {
		return errors.Errorf("grant covers Space %q, not %q", g.GetSpaceId(), spaceID)
	}
	if g.GetPackBaseUrl() == "" {
		return errors.New("grant names no pack base URL")
	}
	if !now.Before(time.UnixMilli(g.GetExpiresAtMs())) {
		return errors.New("grant has expired")
	}
	return nil
}

// GrantKeeper holds the current grant of one Space and renews it when three
// quarters of its life have passed, so pack reads never wait on a renewal
// while the current grant is valid. A writer calls Current before each write,
// which keeps its admission, renewed with the grant, from lapsing.
type GrantKeeper struct {
	src     GrantSource
	spaceID string
	now     func() time.Time

	mtx sync.Mutex
	// grant is the current grant, or nil.
	grant *cdn.CdnSpaceGrant
	// renewAt is when the next read asks for a fresh grant.
	renewAt time.Time
	// load is the renewal in flight, or nil.
	load *grantLoad
}

// grantLoad is one renewal its waiters share.
type grantLoad struct {
	done chan struct{}
	err  error
}

// NewGrantKeeper returns a keeper asking src for grants of spaceID.
func NewGrantKeeper(src GrantSource, spaceID string) *GrantKeeper {
	return &GrantKeeper{src: src, spaceID: spaceID, now: time.Now}
}

// Current returns a valid grant. It renews the grant when it is due, and
// waits on the renewal only when the grant it holds is no longer valid.
func (k *GrantKeeper) Current(ctx context.Context) (*cdn.CdnSpaceGrant, error) {
	for {
		// Return a grant that is not yet due, or a valid one while another
		// read renews it.
		k.mtx.Lock()
		now := k.now()
		valid := k.grant != nil && now.Before(time.UnixMilli(k.grant.GetExpiresAtMs()))
		if valid && (now.Before(k.renewAt) || k.load != nil) {
			g := k.grant
			k.mtx.Unlock()
			return g, nil
		}

		// Wait on the renewal in flight when the held grant cannot serve.
		if load := k.load; load != nil {
			k.mtx.Unlock()
			select {
			case <-load.done:
			case <-ctx.Done():
				return nil, context.Cause(ctx)
			}
			if load.err != nil {
				return nil, load.err
			}
			continue
		}

		// Renew the grant; a valid one keeps serving if the renewal fails.
		load := &grantLoad{done: make(chan struct{})}
		k.load = load
		k.mtx.Unlock()
		g, err := k.renew(ctx)
		k.finish(load, g, err)
		if err != nil && !valid {
			return nil, err
		}
	}
}

// renew asks the source for a fresh grant and checks it.
func (k *GrantKeeper) renew(ctx context.Context) (*cdn.CdnSpaceGrant, error) {
	g, err := k.src.SpaceGrant(ctx, k.spaceID)
	if err != nil {
		return nil, errors.Wrap(err, "renew CDN Space grant")
	}
	if err := ValidateGrant(g, k.spaceID, k.now()); err != nil {
		return nil, err
	}
	return g, nil
}

// finish records a renewal's result and wakes its waiters.
func (k *GrantKeeper) finish(load *grantLoad, g *cdn.CdnSpaceGrant, err error) {
	// Keep the grant until three quarters of its life, or retry a failure soon.
	k.mtx.Lock()
	defer k.mtx.Unlock()
	now := k.now()
	k.renewAt = now.Add(grantRetry)
	if err == nil {
		life := time.UnixMilli(g.GetExpiresAtMs()).Sub(now)
		k.grant, k.renewAt = g, now.Add(life*3/4)
	}

	// Wake the reads waiting on this renewal.
	load.err = err
	k.load = nil
	close(load.done)
}

// refused marks g as refused by the bucket, so the next read renews it.
func (k *GrantKeeper) refused(g *cdn.CdnSpaceGrant) {
	k.mtx.Lock()
	defer k.mtx.Unlock()
	if k.grant == g {
		k.grant = nil
	}
}

// Opener returns an opener whose pack reads go to the current grant's pack
// base with its authorization. A read the bucket refuses with 401 or 403
// drops that grant, so the next read asks for a fresh one.
func (k *GrantKeeper) Opener(httpCli *http.Client) packfile_store.Opener {
	if httpCli == nil {
		httpCli = http.DefaultClient
	}
	spaceID := k.spaceID
	return func(packID string, size int64) (*packfile_store.PackReader, error) {
		// Require the size from the manifest, as the anonymous opener does.
		if size <= 0 {
			return nil, errors.New("pack size must be known from the manifest")
		}

		// Point each range request at the current grant.
		var mtx sync.Mutex
		var sent *cdn.CdnSpaceGrant
		sign := func(req *http.Request) error {
			// Send the request to the current grant's pack URL with its token.
			g, err := k.Current(req.Context())
			if err != nil {
				return err
			}
			u, err := url.Parse(packURL(g.GetPackBaseUrl(), spaceID, packID))
			if err != nil {
				return errors.Wrap(err, "parse granted pack URL")
			}
			req.URL, req.Host = u, ""
			if auth := g.GetAuthorization(); auth != "" {
				req.Header.Set("Authorization", auth)
			}

			// Remember the grant sent, which a refusal drops.
			mtx.Lock()
			sent = g
			mtx.Unlock()
			return nil
		}

		// Drop a grant the bucket refuses.
		observe := func(resp *http.Response) {
			// Drop the grant this request sent if the bucket refused it.
			if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
				return
			}
			mtx.Lock()
			g := sent
			mtx.Unlock()
			k.refused(g)
		}
		return packfile_store.NewHTTPRangeReader(httpCli, packURL("grant:", spaceID, packID), size, readAheadSize, sign, observe), nil
	}
}
