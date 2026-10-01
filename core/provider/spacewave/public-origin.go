package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// LinkPublicOrigin links an owner-paid bucket as the public Space's pack origin
// and returns the origin's status. The cloud proves that link's public base URL
// serves the bucket before storing it, then copies the Space's existing packs.
func (c *SessionClient) LinkPublicOrigin(
	ctx context.Context,
	spaceID string,
	link *s4wave_provider_spacewave.PublicOriginLink,
) (*s4wave_provider_spacewave.PublicOriginStatus, error) {
	// Encode the link, which carries the bucket's secret key.
	body, err := link.MarshalVT()
	if err != nil {
		return nil, err
	}

	// Post it; the cloud proves the origin before storing it.
	data, err := c.doPostBinary(ctx, publicOriginPath(spaceID), body, nil, SeedReasonMutation)
	if err != nil {
		return nil, err
	}
	return unmarshalPublicOriginStatus(data)
}

// GetPublicOrigin returns the public Space's linked origin status.
func (c *SessionClient) GetPublicOrigin(
	ctx context.Context,
	spaceID string,
) (*s4wave_provider_spacewave.PublicOriginStatus, error) {
	data, err := c.doGetBinary(ctx, publicOriginPath(spaceID), SeedReasonColdSeed)
	if err != nil {
		return nil, err
	}
	return unmarshalPublicOriginStatus(data)
}

// ReleaseHostedCopies deletes Spacewave's copies of a public Space that moved to
// its linked origin and returns the origin's status.
func (c *SessionClient) ReleaseHostedCopies(
	ctx context.Context,
	spaceID string,
) (*s4wave_provider_spacewave.PublicOriginStatus, error) {
	data, err := c.doDelete(ctx, publicOriginPath(spaceID)+"/hosted-copies", SeedReasonMutation)
	if err != nil {
		return nil, err
	}
	return unmarshalPublicOriginStatus(data)
}

// publicOriginPath returns the cloud route for a Space's linked origin.
func publicOriginPath(spaceID string) string {
	return "/api/sobject/" + spaceID + "/public-origin"
}

// unmarshalPublicOriginStatus decodes a public origin route's response body.
func unmarshalPublicOriginStatus(data []byte) (*s4wave_provider_spacewave.PublicOriginStatus, error) {
	status := &s4wave_provider_spacewave.PublicOriginStatus{}
	if err := status.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal public origin status")
	}
	return status, nil
}
