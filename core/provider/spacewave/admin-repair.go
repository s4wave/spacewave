package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider/spacewave/adminrepair"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// ApplyPackMetadataRepair applies verified pack metadata repairs.
func (c *SessionClient) ApplyPackMetadataRepair(
	ctx context.Context,
	resourceID string,
	req *api.PackMetadataRepairRequest,
) (*api.PackMetadataRepairResponse, error) {
	// Reject an empty repair request.
	if req == nil {
		return nil, errors.New("pack metadata repair request is nil")
	}

	// Marshal the repair request into a binary body.
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal pack metadata repair request")
	}

	// Post the body to the pack metadata repair route.
	respBody, err := c.doPostBinary(
		ctx,
		adminrepair.Path(resourceID),
		body,
		nil,
		SeedReasonMutation,
	)
	if err != nil {
		return nil, err
	}

	// Unmarshal the cloud response.
	resp := &api.PackMetadataRepairResponse{}
	if err := resp.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal pack metadata repair response")
	}
	return resp, nil
}
