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

// SetApplicationFunding accepts a payer through application and billing administration.
// Retrying identical input returns its original assignment without changing current funding.
func (c *SessionClient) SetApplicationFunding(ctx context.Context, req *api.SetApplicationFundingRequest) (*api.SetApplicationFundingResponse, error) {
	// Encode the payer and revision fence using the shared funding contract.
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal application funding")
	}

	// The authenticated account must hold both application and payer authority.
	data, err := c.doPostBinary(ctx, "/api/applications/funding/set", body, nil, SeedReasonMutation)
	if err != nil {
		return nil, errors.Wrap(err, "set application funding")
	}

	// Preserve the server's accepted interval and authorization provenance.
	var resp api.SetApplicationFundingResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal application funding")
	}
	return &resp, nil
}

// ListApplicationFunding reads one bounded page of accepted funding assignments.
// The server permits the registered operator and platform administrators.
func (c *SessionClient) ListApplicationFunding(ctx context.Context, req *api.ListApplicationFundingRequest) (*api.ListApplicationFundingResponse, error) {
	// Encode the application and continuation cursor with the generated contract.
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal application funding history")
	}

	// Read through the Session's authenticated provider transport.
	data, err := c.doPostBinary(ctx, "/api/applications/funding/list", body, nil, "")
	if err != nil {
		return nil, errors.Wrap(err, "list application funding")
	}

	// Return the immutable assignments and next-page cursor.
	var resp api.ListApplicationFundingResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal application funding history")
	}
	return &resp, nil
}
