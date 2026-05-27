package adminrepair

import (
	"path"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// Path returns the cloud API path for pack metadata repair.
func Path(resourceID string) string {
	return path.Join("/api/admin/bstore", resourceID, "pack-metadata-repair")
}

// MarshalRequest marshals the pack metadata repair request body.
func MarshalRequest(req *api.PackMetadataRepairRequest) ([]byte, error) {
	return req.MarshalVT()
}

// ParseResponse parses the pack metadata repair response body.
func ParseResponse(body []byte) (*api.PackMetadataRepairResponse, error) {
	resp := &api.PackMetadataRepairResponse{}
	if err := resp.UnmarshalVT(body); err != nil {
		return nil, err
	}
	return resp, nil
}
