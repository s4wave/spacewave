//go:build !js

package cli

import (
	"context"
	"os"
	"strconv"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	debug_cli "github.com/s4wave/spacewave/cmd/alpha-debug/cli"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	s4wave_deploy "github.com/s4wave/spacewave/sdk/deploy"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/sirupsen/logrus"
)

const (
	maxWalkDepth = 10

	// engineBucketID is the bucket ID used by the devtool world engine.
	engineBucketID = "bldr/devtool"
	// engineObjStoreID is the object store ID used by the devtool world engine.
	engineObjStoreID = "bldr/devtool"
	// pluginHostObjectKey is the object key for the devtool plugin host.
	pluginHostObjectKey = "devtool"
)

// DeployArgs contains the deploy command arguments.
type DeployArgs struct {
	debug_cli.ClientArgs

	// SpaceID is the ID of the Space to deploy to.
	SpaceID string
	// SourcePath is the path to the .bldr/ directory containing built manifests.
	SourcePath string
	// ManifestID is the manifest identifier.
	ManifestID string
	// ObjectKey is the object key to store the manifest under.
	ObjectKey string
}

// BuildDeployCommand returns the deploy manifest command.
func (a *DeployArgs) BuildDeployCommand() *cli.Command {
	return &cli.Command{
		Name:  "manifest",
		Usage: "deploy a manifest and its blocks into a Space",
		Flags: append(a.BuildFlags(), []cli.Flag{
			&cli.StringFlag{
				Name:        "space-id",
				Usage:       "ID of the Space to deploy to",
				Required:    true,
				Destination: &a.SpaceID,
			},
			&cli.StringFlag{
				Name:        "source",
				Usage:       "path to .bldr/ directory containing built manifests",
				Required:    true,
				Destination: &a.SourcePath,
			},
			&cli.StringFlag{
				Name:        "manifest-id",
				Usage:       "manifest identifier (e.g., glados-core)",
				Required:    true,
				Destination: &a.ManifestID,
			},
			&cli.StringFlag{
				Name:        "object-key",
				Usage:       "object key to store the manifest under in the Space world",
				Value:       "",
				Destination: &a.ObjectKey,
			},
		}...),
		Action: a.RunDeploy,
	}
}

// RunDeploy executes the deploy manifest command.
func (a *DeployArgs) RunDeploy(c *cli.Context) error {
	ctx := c.Context

	objectKey := a.ObjectKey
	if objectKey == "" {
		objectKey = a.ManifestID
	}

	os.Stdout.WriteString("deploying manifest " + a.ManifestID + " to space " + a.SpaceID + " (key=" + objectKey + ")\n")
	os.Stdout.WriteString("source: " + a.SourcePath + "\n")

	le := logrus.NewEntry(logrus.StandardLogger())

	// Open the devtool sqlite volume.
	vol, err := openDevtoolVolume(ctx, le, a.SourcePath)
	if err != nil {
		return errors.Wrap(err, "open devtool storage")
	}
	defer vol.Close()

	sfs := buildStepFactorySet()

	// Build the transform config used by the devtool world.
	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_s2.Config{},
	})
	if err != nil {
		return errors.Wrap(err, "build transform config")
	}

	// Look up the manifest by ID.
	collected, err := lookupManifest(ctx, le, vol, sfs, a.ManifestID)
	if err != nil {
		return errors.Wrap(err, "lookup manifest")
	}

	// Build the full ObjectRef with transform config so the server can decode blocks.
	manifestObjRef := collected.ManifestRef.Clone()
	manifestObjRef.TransformConf = transformConf

	os.Stdout.WriteString("found manifest " + a.ManifestID +
		" rev=" + strconv.FormatUint(uint64(collected.GetRev()), 10) +
		" ref=" + manifestObjRef.GetRootRef().MarshalString() + "\n")

	// Navigate to the Space via Resource SDK through PluginRpc.
	spaceSvc, cleanup, err := a.mountSpaceResource(ctx)
	if err != nil {
		return errors.Wrap(err, "mount space resource")
	}
	defer cleanup()

	// Open bidirectional deploy stream on the space resource.
	strm, err := spaceSvc.DeployManifest(ctx)
	if err != nil {
		return errors.Wrap(err, "open deploy stream")
	}

	// Send initial deploy request with full ObjectRef (includes transform config).
	err = strm.Send(&s4wave_deploy.DeployManifestMessage{
		Body: &s4wave_deploy.DeployManifestMessage_Request{
			Request: &s4wave_deploy.DeployManifestRequest{
				SpaceId:     a.SpaceID,
				ManifestRef: manifestObjRef,
				ObjectKey:   objectKey,
				ManifestId:  a.ManifestID,
			},
		},
	})
	if err != nil {
		return errors.Wrap(err, "send deploy request")
	}

	// Handle block exchange loop.
	return a.blockExchangeLoop(ctx, strm, vol)
}

// mountSpaceResource navigates the Resource SDK to reach the SpaceResourceService
// for the target space: core plugin -> root -> session -> shared object -> body.
func (a *DeployArgs) mountSpaceResource(ctx context.Context) (s4wave_space.SRPCSpaceResourceServiceClient, func(), error) {
	coreClient, err := a.BuildCoreClient()
	if err != nil {
		return nil, nil, errors.Wrap(err, "connect to core plugin")
	}

	resourceSvc := resource.NewSRPCResourceServiceClient(coreClient)
	resClient, err := resource_client.NewClient(ctx, resourceSvc)
	if err != nil {
		return nil, nil, errors.Wrap(err, "resource client")
	}

	rootRef := resClient.AccessRootResource()
	root, err := s4wave_root.NewRoot(resClient, rootRef)
	if err != nil {
		resClient.Release()
		return nil, nil, errors.Wrap(err, "root resource")
	}

	resp, err := root.MountSessionByIdx(ctx, uint32(a.SessionIdx))
	if err != nil {
		root.Release()
		resClient.Release()
		return nil, nil, errors.Wrap(err, "mount session")
	}
	if resp.GetNotFound() {
		root.Release()
		resClient.Release()
		return nil, nil, errors.Errorf("no session at index %d", a.SessionIdx)
	}

	sessRef := resClient.CreateResourceReference(resp.GetResourceId())
	sess, err := s4wave_session.NewSession(resClient, sessRef)
	if err != nil {
		sessRef.Release()
		root.Release()
		resClient.Release()
		return nil, nil, errors.Wrap(err, "session resource")
	}

	// Mount the shared object for the space.
	soResp, err := sess.MountSharedObject(ctx, a.SpaceID)
	if err != nil {
		sess.Release()
		root.Release()
		resClient.Release()
		return nil, nil, errors.Wrap(err, "mount shared object")
	}

	soRef := resClient.CreateResourceReference(soResp.GetResourceId())
	soClient, err := soRef.GetClient()
	if err != nil {
		soRef.Release()
		sess.Release()
		root.Release()
		resClient.Release()
		return nil, nil, errors.Wrap(err, "shared object client")
	}

	// Mount the shared object body (returns SpaceResource).
	soSvc := s4wave_sobject.NewSRPCSharedObjectResourceServiceClient(soClient)
	bodyResp, err := soSvc.MountSharedObjectBody(ctx, &s4wave_sobject.MountSharedObjectBodyRequest{})
	if err != nil {
		soRef.Release()
		sess.Release()
		root.Release()
		resClient.Release()
		return nil, nil, errors.Wrap(err, "mount shared object body")
	}

	bodyRef := resClient.CreateResourceReference(bodyResp.GetResourceId())
	bodyClient, err := bodyRef.GetClient()
	if err != nil {
		bodyRef.Release()
		soRef.Release()
		sess.Release()
		root.Release()
		resClient.Release()
		return nil, nil, errors.Wrap(err, "space body client")
	}

	spaceSvc := s4wave_space.NewSRPCSpaceResourceServiceClient(bodyClient)

	cleanup := func() {
		bodyRef.Release()
		soRef.Release()
		sess.Release()
		root.Release()
		resClient.Release()
	}
	return spaceSvc, cleanup, nil
}

// blockExchangeLoop handles the block request/response exchange with the server.
func (a *DeployArgs) blockExchangeLoop(
	ctx context.Context,
	strm s4wave_space.SRPCSpaceResourceService_DeployManifestClient,
	vol volume.Volume,
) error {
	for {
		msg, err := strm.Recv()
		if err != nil {
			return errors.Wrap(err, "recv from server")
		}

		switch body := msg.GetBody().(type) {
		case *s4wave_deploy.DeployManifestMessage_BlockRequest:
			ref := body.BlockRequest.GetRef()
			os.Stdout.WriteString("server requested block: " + ref.MarshalString() + "\n")

			data, found, err := vol.GetBlock(ctx, ref)
			if err != nil {
				return errors.Wrap(err, "read block from storage")
			}

			resp := &s4wave_deploy.BlockResponse{
				Ref:      ref,
				NotFound: !found,
			}
			if found {
				resp.Data = data
			}
			err = strm.Send(&s4wave_deploy.DeployManifestMessage{
				Body: &s4wave_deploy.DeployManifestMessage_BlockResponse{
					BlockResponse: resp,
				},
			})
			if err != nil {
				return errors.Wrap(err, "send block response")
			}

		case *s4wave_deploy.DeployManifestMessage_Result:
			result := body.Result
			if result.GetError() != "" {
				return errors.Errorf("deploy failed: %s", result.GetError())
			}
			os.Stdout.WriteString("deploy complete\n")
			return nil

		default:
			return errors.Errorf("unexpected message from server: %T", body)
		}
	}
}

// lookupManifest opens the devtool world and finds a manifest by ID.
func lookupManifest(
	ctx context.Context,
	le *logrus.Entry,
	vol volume.Volume,
	sfs *block_transform.StepFactorySet,
	manifestID string,
) (*bldr_manifest_world.CollectedManifest, error) {
	headRef, err := loadHeadRef(ctx, vol)
	if err != nil {
		return nil, errors.Wrap(err, "load head ref")
	}
	if headRef.GetRootRef().GetEmpty() {
		return nil, errors.New("devtool world is empty (no head ref)")
	}

	if headRef.GetBucketId() == "" {
		headRef.BucketId = engineBucketID
	}

	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_s2.Config{},
	})
	if err != nil {
		return nil, errors.Wrap(err, "build transform config")
	}

	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: le},
		sfs,
		transformConf,
	)
	if err != nil {
		return nil, errors.Wrap(err, "build block transformer")
	}

	cursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		le,
		sfs,
		vol,
		xfrm,
		headRef,
		&bucket.BucketOpArgs{
			BucketId: engineBucketID,
		},
		transformConf,
	)

	eng, err := world_block.NewEngine(
		ctx,
		le,
		cursor,
		bldr_manifest_world.LookupOp,
		nil,
		false,
	)
	if err != nil {
		return nil, errors.Wrap(err, "build world engine")
	}

	ws := world.NewEngineWorldState(eng, false)

	manifests, _, err := bldr_manifest_world.CollectManifests(ctx, ws, nil, pluginHostObjectKey)
	if err != nil {
		return nil, errors.Wrap(err, "collect manifests")
	}

	list, ok := manifests[manifestID]
	if !ok || len(list) == 0 {
		available := make([]string, 0, len(manifests))
		for id := range manifests {
			available = append(available, id)
		}
		return nil, errors.Errorf("manifest %q not found (available: %v)", manifestID, available)
	}

	return list[0], nil
}
