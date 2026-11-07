// Package cdn_grant issues CdnSpaceGrants for the private CDN Spaces a
// writer owns. A grant pairs a bucket download authorization scoped to one
// Space's prefix with, when the holder may write, a writer admission for the
// same span, so the holder reads and publishes straight to the bucket and the
// server never relays pack bytes.
//
// A volume serves any block it holds to anyone who names its hash, so a
// server holding public and private content keeps them on separate volumes.
package cdn_grant

import (
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/cdn"
	cdn_writer "github.com/s4wave/spacewave/core/cdn/writer"
	"github.com/s4wave/spacewave/net/peer"
)

// DefaultTTL is how long a grant lasts when the issuer sets no TTL.
const DefaultTTL = 12 * time.Hour

// DownloadAuthorization lets its holder read the bucket objects under one key
// prefix until it expires.
type DownloadAuthorization struct {
	// BaseURL reads the object with key k at BaseURL + "/" + k.
	BaseURL string
	// Authorization is the Authorization header value each read sends.
	Authorization string
	// Expires is when the authorization ends.
	Expires time.Time
}

// DownloadAuthorizer mints bucket download authorizations scoped to a key
// prefix, such as B2's native download authorization.
type DownloadAuthorizer interface {
	// AuthorizeDownloads authorizes reads of the keys under prefix for ttl.
	AuthorizeDownloads(ctx context.Context, prefix string, ttl time.Duration) (*DownloadAuthorization, error)
}

// Issuer issues grants for the Spaces Writer owns.
type Issuer struct {
	// Writer owns the Spaces and admits the writers grants allow.
	Writer *cdn_writer.Writer
	// Downloads mints the download authorizations.
	Downloads DownloadAuthorizer
	// TTL is each grant's life. Zero selects DefaultTTL.
	TTL time.Duration
	// Now reads the clock. Nil selects time.Now.
	Now func() time.Time
}

// Issue returns a grant for holder to read spaceID, and to publish to it when
// mayWrite is set. The app decides who may hold which grant.
func (i *Issuer) Issue(ctx context.Context, spaceID string, holder peer.ID, mayWrite bool) (*cdn.CdnSpaceGrant, error) {
	// Require an owned Space and a holder to admit.
	if i.Writer.Root(spaceID) == nil {
		return nil, cdn_writer.ErrSpaceUnknown
	}
	if mayWrite && holder == "" {
		return nil, errors.New("a writing grant needs a holder peer")
	}
	ttl := i.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	now := time.Now
	if i.Now != nil {
		now = i.Now
	}

	// Authorize reads of the Space's prefix, and find the pack base under it.
	prefix := i.Writer.SpacePrefix(spaceID)
	parent := strings.TrimSuffix(prefix, spaceID+"/")
	if parent != "" && !strings.HasSuffix(parent, "/") {
		return nil, errors.Errorf("writer prefix %q must be empty or end in /", parent)
	}
	auth, err := i.Downloads.AuthorizeDownloads(ctx, prefix, ttl)
	if err != nil {
		return nil, errors.Wrap(err, "authorize Space downloads")
	}
	expires := now().Add(ttl)
	if auth.Expires.Before(expires) {
		expires = auth.Expires
	}

	// Admit the holder as a writer for the grant's span.
	if mayWrite {
		if err := i.Writer.Admit(spaceID, holder, expires); err != nil {
			return nil, err
		}
	}
	return &cdn.CdnSpaceGrant{
		SpaceId:       spaceID,
		PackBaseUrl:   strings.TrimSuffix(strings.TrimSuffix(auth.BaseURL, "/")+"/"+parent, "/"),
		Authorization: auth.Authorization,
		ExpiresAtMs:   expires.UnixMilli(),
		MayWrite:      mayWrite,
	}, nil
}
