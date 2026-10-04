package s4wave_world

import "context"

// AccessTypedObject acquires a typed child and restores the response's typed
// error. Retirement of the exact granting mount matches ErrTypedObjectGrantRetired
// with errors.Is. Caller cancellation and transport failures remain terminal.
func AccessTypedObject(ctx context.Context, service SRPCTypedObjectResourceServiceClient, req *AccessTypedObjectRequest) (*AccessTypedObjectResponse, error) {
	// Request the child through the granting mount's Resource RPC client.
	response, err := service.AccessTypedObject(ctx, req)
	if err != nil {
		return nil, err
	}

	// Restore the acquisition failure before exposing a child reference.
	if err := response.GetError(); err != nil {
		if callerErr := ctx.Err(); callerErr != nil {
			return nil, callerErr
		}
		return nil, err
	}
	return response, nil
}

// GetError restores the typed acquisition failure carried by the response.
func (r *AccessTypedObjectResponse) GetError() error {
	return ErrorFromCode(r.GetErrorCode())
}
