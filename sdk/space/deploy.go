package s4wave_space

import (
	"context"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	s4wave_deploy "github.com/s4wave/spacewave/sdk/deploy"
)

// DeployManifests stores refs under objectKey in the Space behind svc. The
// Space requests each block it lacks, and getBlock answers from the source
// store that holds the manifests, such as a devtool volume's GetBlock. onBlock,
// when set, sees each requested ref before it is read.
func DeployManifests(
	ctx context.Context,
	svc SRPCSpaceResourceServiceClient,
	objectKey string,
	refs []*bldr_manifest.ManifestRef,
	getBlock func(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error),
	onBlock func(ref *block.BlockRef),
) error {
	// Open the stream and send the request.
	strm, err := svc.DeployManifests(ctx)
	if err != nil {
		return errors.Wrap(err, "open deploy stream")
	}
	defer strm.Close()
	err = strm.Send(&s4wave_deploy.DeployManifestsMessage{
		Body: &s4wave_deploy.DeployManifestsMessage_Request{
			Request: &s4wave_deploy.DeployManifestsRequest{ObjectKey: objectKey, ManifestRefs: refs},
		},
	})
	if err != nil {
		return errors.Wrap(err, "send deploy request")
	}

	// Answer block requests until the Space sends the result.
	for {
		msg, err := strm.Recv()
		if err != nil {
			return errors.Wrap(err, "recv from server")
		}
		switch body := msg.GetBody().(type) {
		case *s4wave_deploy.DeployManifestsMessage_BlockRequest:
			// Read the requested block and send it, or report it missing.
			ref := body.BlockRequest.GetRef()
			if onBlock != nil {
				onBlock(ref)
			}
			data, found, err := getBlock(ctx, ref)
			if err != nil {
				return errors.Wrap(err, "read block from source")
			}
			err = strm.Send(&s4wave_deploy.DeployManifestsMessage{
				Body: &s4wave_deploy.DeployManifestsMessage_BlockResponse{
					BlockResponse: &s4wave_deploy.BlockResponse{Ref: ref, NotFound: !found, Data: data},
				},
			})
			if err != nil {
				return errors.Wrap(err, "send block response")
			}

		case *s4wave_deploy.DeployManifestsMessage_Result:
			// Return the Space's verdict.
			if msg := body.Result.GetError(); msg != "" {
				return errors.Errorf("deploy failed: %s", msg)
			}
			return nil

		default:
			return errors.Errorf("unexpected message from server: %T", body)
		}
	}
}
