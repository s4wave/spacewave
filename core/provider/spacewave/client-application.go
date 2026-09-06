package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// RegisterApplication registers a paused product under platform administration.
// Reusing the request ID with identical input returns the existing registration.
func (c *SessionClient) RegisterApplication(ctx context.Context, req *api.RegisterApplicationRequest) (*api.RegisterApplicationResponse, error) {
	// Encode the registration with the shared application contract.
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal application registration")
	}

	// Authenticate the mutation using this Session's signing identity.
	data, err := c.doPostBinary(ctx, "/api/applications/register", body, nil, SeedReasonMutation)
	if err != nil {
		return nil, errors.Wrap(err, "register application")
	}

	// Return the persisted registration, including on an idempotent retry.
	var resp api.RegisterApplicationResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal application registration")
	}
	return &resp, nil
}

// GetApplication reads a product registration visible to this Session's account.
// The server permits the registered operator and platform administrators.
func (c *SessionClient) GetApplication(ctx context.Context, applicationID string) (*api.GetApplicationResponse, error) {
	// Encode the selected registration without any caller identity override.
	body, err := (&api.GetApplicationRequest{ApplicationId: applicationID}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal application lookup")
	}

	// Read through the same authenticated transport as account operations.
	data, err := c.doPostBinary(ctx, "/api/applications/get", body, nil, "")
	if err != nil {
		return nil, errors.Wrap(err, "get application")
	}

	// Decode the current registration using the generated response contract.
	var resp api.GetApplicationResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal application lookup")
	}
	return &resp, nil
}
