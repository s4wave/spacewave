package s4wave_sobject

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
)

// MountSharedObjectBody returns a mounted body or its typed health failure.
// Callers must receive a successful result before creating a Resource reference.
func MountSharedObjectBody(ctx context.Context, service SRPCSharedObjectResourceServiceClient) (*MountSharedObjectBodyResponse, error) {
	response, err := service.MountSharedObjectBody(ctx, &MountSharedObjectBodyRequest{})
	if err != nil {
		return nil, err
	}
	if health := response.GetHealth(); health != nil {
		return nil, sobject.NewSharedObjectHealthError(health, nil)
	}
	return response, nil
}
