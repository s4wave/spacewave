package resource_client

import (
	"context"
	"errors"

	"github.com/aperturerobotics/starpc/srpc"
	pkgerrors "github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
)

type adoptedResourceIDsResponse interface {
	GetAdoptedResourceIds() []uint32
}

type adoptingResourceRPCClient struct {
	ctx            context.Context
	client         srpc.Client
	service        resource.SRPCResourceServiceClient
	clientHandleID uint32
}

func (c *adoptingResourceRPCClient) ExecCall(
	ctx context.Context,
	serviceID string,
	methodID string,
	in srpc.Message,
	out srpc.Message,
) error {
	if !resource.IsResourceRPCAdoptingUnaryMethod(serviceID, methodID) {
		return c.client.ExecCall(ctx, serviceID, methodID, in, out)
	}

	receipt, err := srpc.ExecCallReceipt(ctx, c.client, serviceID, methodID, in, out)
	if err != nil {
		return err
	}
	defer receipt.Close()

	response, ok := out.(adoptedResourceIDsResponse)
	if !ok {
		responseErr := errors.New("resource RPC response lacks adoption metadata")
		if abortErr := receipt.Abort(); abortErr != nil {
			return errors.Join(responseErr, pkgerrors.Wrap(abortErr, "abort resource RPC receipt"))
		}
		return responseErr
	}
	resourceIDs := response.GetAdoptedResourceIds()

	adopted := make(map[uint32]struct{}, len(resourceIDs))
	for _, resourceID := range resourceIDs {
		if resourceID == 0 {
			continue
		}
		if _, ok := adopted[resourceID]; ok {
			continue
		}
		adopted[resourceID] = struct{}{}

		_, err := c.service.ResourceRefAdopt(ctx, &resource.ResourceRefAdoptRequest{
			ClientHandleId: c.clientHandleID,
			ResourceId:     resourceID,
		})
		if err != nil {
			adoptErr := pkgerrors.Wrap(err, "adopt returned resource")
			if abortErr := receipt.Abort(); abortErr != nil {
				return errors.Join(adoptErr, pkgerrors.Wrap(abortErr, "abort resource RPC receipt"))
			}
			return adoptErr
		}
	}

	if err := receipt.Commit(); err != nil {
		commitErr := pkgerrors.Wrap(err, "commit resource RPC receipt")
		for resourceID := range adopted {
			_, releaseErr := c.service.ResourceRefRelease(c.ctx, &resource.ResourceRefReleaseRequest{
				ClientHandleId: c.clientHandleID,
				ResourceId:     resourceID,
			})
			if releaseErr != nil {
				commitErr = errors.Join(
					commitErr,
					pkgerrors.Wrap(releaseErr, "release resource after commit failure"),
				)
			}
		}
		return commitErr
	}
	return nil
}

func (c *adoptingResourceRPCClient) NewStream(
	ctx context.Context,
	serviceID string,
	methodID string,
	firstMsg srpc.Message,
) (srpc.Stream, error) {
	return c.client.NewStream(ctx, serviceID, methodID, firstMsg)
}

// _ is a type assertion
var _ srpc.Client = ((*adoptingResourceRPCClient)(nil))
