package resource_session

import (
	"context"

	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// TransferResource transfers a resource to a typed principal owner.
func (r *SpacewaveSessionResource) TransferResource(
	ctx context.Context,
	req *s4wave_provider_spacewave.TransferResourceRequest,
) (*s4wave_provider_spacewave.TransferResourceResponse, error) {
	// Resolve the current signer before the cloud request.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	_, err = cli.TransferResource(
		ctx,
		req.GetResourceId(),
		req.GetNewOwnerType(),
		req.GetNewOwnerId(),
	)
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.TransferResourceResponse{}, nil
}

// RepairSharedObject retries owner-side repair for a broken shared object.
func (r *SpacewaveSessionResource) RepairSharedObject(
	ctx context.Context,
	req *s4wave_provider_spacewave.RepairSharedObjectRequest,
) (*s4wave_provider_spacewave.RepairSharedObjectResponse, error) {
	// Require the shared object to repair.
	sharedObjectID := req.GetSharedObjectId()
	if sharedObjectID == "" {
		return nil, errors.New("shared object id is required")
	}

	// Repair it, reporting a missing recovery credential to the caller.
	err := r.swAcc.RepairSharedObject(ctx, sharedObjectID)
	if errors.Is(err, sobject.ErrSharedObjectRecoveryCredentialRequired) {
		return &s4wave_provider_spacewave.RepairSharedObjectResponse{CredentialRequired: true}, nil
	}
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.RepairSharedObjectResponse{}, nil
}

// ReinitializeSharedObject destructively rewrites a broken shared object in place.
func (r *SpacewaveSessionResource) ReinitializeSharedObject(
	ctx context.Context,
	req *s4wave_provider_spacewave.ReinitializeSharedObjectRequest,
) (*s4wave_provider_spacewave.ReinitializeSharedObjectResponse, error) {
	// Require a SharedObject ID before rewriting broken provider state.
	sharedObjectID := req.GetSharedObjectId()
	if sharedObjectID == "" {
		return nil, errors.New("shared object id is required")
	}
	if err := r.swAcc.ReinitializeSharedObject(ctx, sharedObjectID); err != nil {
		return nil, err
	}
	r.refreshOrganizationCaches(ctx, sharedObjectID, true)
	return &s4wave_provider_spacewave.ReinitializeSharedObjectResponse{}, nil
}

// MountSharedObjectSelfEnrollment mounts the self-enrollment resource.
func (r *SpacewaveSessionResource) MountSharedObjectSelfEnrollment(
	ctx context.Context,
	req *s4wave_session.MountSharedObjectSelfEnrollmentRequest,
) (*s4wave_session.MountSharedObjectSelfEnrollmentResponse, error) {
	// Resolve the caller resource context and register self-enrollment.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	res := NewSharedObjectSelfEnrollmentResource(r.swAcc)
	id, err := resourceCtx.AddResource(res.GetMux(), func() {})
	if err != nil {
		return nil, err
	}
	return &s4wave_session.MountSharedObjectSelfEnrollmentResponse{ResourceId: id}, nil
}

// mountSpaceSO mounts a space shared object by ID and returns the typed SO.
// Caller must defer releaseFn.
func (r *SpacewaveSessionResource) mountSpaceSO(
	ctx context.Context,
	spaceID string,
) (*provider_spacewave.SharedObject, func(), error) {
	// Build the provider-scoped Space reference before mounting it.
	ref := sobject.NewSharedObjectRef(
		r.swAcc.GetProviderID(),
		r.swAcc.GetAccountID(),
		spaceID,
		provider_spacewave.SobjectBlockStoreID(spaceID),
	)
	so, rel, err := r.swAcc.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "mount shared object")
	}
	swSO, ok := so.(*provider_spacewave.SharedObject)
	if !ok {
		rel()
		return nil, nil, errors.New("unexpected shared object type")
	}
	return swSO, rel, nil
}
