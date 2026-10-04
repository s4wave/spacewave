package provider_spacewave

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
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

// OpenPackReader opens a pack of a block store for HTTP Range reads on its
// granted read URLs, renewing the grant before it expires. The size comes
// from the manifest entry, so no HEAD request is issued.
func (c *SessionClient) OpenPackReader(resourceID, packID string, size int64) (*packfile_store.PackReader, error) {
	if size <= 0 {
		return nil, errors.New("pack size must be known from the manifest")
	}
	return packfile_store.NewHTTPRangeReader(
		c.httpCli,
		"",
		size,
		httpReaderAtReadAheadSize,
		func(req *http.Request) error {
			return c.grantPackRead(req, resourceID, packID)
		},
		func(resp *http.Response) {
			c.observePackRead(resp, resourceID, packID)
		},
	), nil
}

// ReadPack opens the whole body of a pack of a block store through its
// granted read URL. The caller closes the body.
func (c *SessionClient) ReadPack(ctx context.Context, resourceID, packID string) (io.ReadCloser, error) {
	// Resolve the pack's read URL.
	readURL, err := c.packReadURL(ctx, resourceID, packID)
	if err != nil {
		return nil, err
	}

	// Fetch the pack and require a successful response.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, readURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "build pack request")
	}
	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "request pack")
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		c.observePackRead(resp, resourceID, packID)
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, errors.Errorf("pack status %d: %s", resp.StatusCode, string(body))
	}
	return resp.Body, nil
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
// download authorization with 401, and other origins refuse with 403. A pack
// that moved to the public bucket when its Space became public answers its
// old URL with 404.
func (c *SessionClient) observePackRead(resp *http.Response, resourceID, packID string) {
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
	default:
		return
	}
	c.readGrantsMtx.Lock()
	delete(c.readGrants, readGrantKey{resourceID: resourceID, packID: packID})
	c.readGrantsMtx.Unlock()
}
