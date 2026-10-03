package provider_spacewave

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile"
)

// readGrantRefresh is how long before its expiry a read grant is replaced, so
// a range request in flight does not outlive its grant.
const readGrantRefresh = time.Minute

// readGrantKey identifies a cached read grant.
type readGrantKey struct {
	// resourceID is the block store ID.
	resourceID string
	// packID is the pack ID.
	packID string
}

// ReadGrants asks the cloud for read URLs of packs in a block store. The
// grants follow the order of packIDs.
func (c *SessionClient) ReadGrants(ctx context.Context, resourceID string, packIDs []string) ([]*packfile.ReadGrant, error) {
	// Request the grants.
	body, err := (&packfile.ReadRequest{PackIds: packIDs}).MarshalVT()
	if err != nil {
		return nil, err
	}
	data, err := c.doPostBinary(ctx, path.Join("/api/bstore", resourceID, "read"), body, nil, SeedReasonColdSeed)
	if err != nil {
		return nil, errors.Wrap(err, "read grants")
	}

	// Decode one grant per pack.
	resp := &packfile.ReadResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal read response")
	}
	if len(resp.GetGrants()) != len(packIDs) {
		return nil, errors.Errorf("read response has %d grants for %d packs", len(resp.GetGrants()), len(packIDs))
	}
	return resp.GetGrants(), nil
}

// packReadURL returns a read URL of a pack valid for at least
// readGrantRefresh, reusing a cached grant when one is.
func (c *SessionClient) packReadURL(ctx context.Context, resourceID, packID string) (string, error) {
	// Reuse a live grant.
	key := readGrantKey{resourceID: resourceID, packID: packID}
	now := time.Now()
	c.readGrantsMtx.Lock()
	grant := c.readGrants[key]
	c.readGrantsMtx.Unlock()
	if grant != nil && grant.GetExpiresAt().AsTime().Sub(now) > readGrantRefresh {
		return grant.GetUrl(), nil
	}

	// Ask for a new grant and forget the expired ones.
	grants, err := c.ReadGrants(ctx, resourceID, []string{packID})
	if err != nil {
		return "", err
	}
	c.readGrantsMtx.Lock()
	defer c.readGrantsMtx.Unlock()
	if c.readGrants == nil {
		c.readGrants = make(map[readGrantKey]*packfile.ReadGrant)
	}
	for k, g := range c.readGrants {
		if !g.GetExpiresAt().AsTime().After(now) {
			delete(c.readGrants, k)
		}
	}
	c.readGrants[key] = grants[0]
	return grants[0].GetUrl(), nil
}

// grantPackRead points a range request at a live read URL of its pack.
func (c *SessionClient) grantPackRead(req *http.Request, resourceID, packID string) error {
	// Resolve the pack's read URL.
	readURL, err := c.packReadURL(req.Context(), resourceID, packID)
	if err != nil {
		return err
	}

	// Send the request there.
	u, err := url.Parse(readURL)
	if err != nil {
		return errors.Wrap(err, "parse read URL")
	}
	req.URL, req.Host = u, u.Host
	return nil
}

// observePackRead forgets the grant of a refused range request, so the next
// request asks for a new one. The bucket answers an expired or invalid
// download authorization with 401, and other origins refuse with 403.
func (c *SessionClient) observePackRead(resp *http.Response, resourceID, packID string) {
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		return
	}
	c.readGrantsMtx.Lock()
	delete(c.readGrants, readGrantKey{resourceID: resourceID, packID: packID})
	c.readGrantsMtx.Unlock()
}
