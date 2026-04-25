package provider_spacewave

import (
	"context"
	"path"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// ApplyPackMetadataRepair applies verified pack metadata repairs.
func (c *SessionClient) ApplyPackMetadataRepair(
	ctx context.Context,
	resourceID string,
	req *api.PackMetadataRepairRequest,
) (*api.PackMetadataRepairResponse, error) {
	if req == nil {
		return nil, errors.New("pack metadata repair request is nil")
	}
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal pack metadata repair request")
	}
	respBody, err := c.doPostBinary(
		ctx,
		path.Join("/api/admin/bstore", resourceID, "pack-metadata-repair"),
		body,
		nil,
		SeedReasonMutation,
	)
	if err != nil {
		return nil, err
	}
	resp := &api.PackMetadataRepairResponse{}
	if err := resp.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal pack metadata repair response")
	}
	return resp, nil
}
