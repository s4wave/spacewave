//go:build js

package main

import (
	"bytes"
	"context"
	stderrors "errors"
	"io"
	"time"

	cbconfig "github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	csp "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_unixfs "github.com/s4wave/spacewave/core/resource/unixfs"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/core"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	unixfs_sdk "github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/volume"
	volume_browser "github.com/s4wave/spacewave/db/volume/browser"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
	"github.com/sirupsen/logrus"
)

// runWorldLargeUnixFSUpload writes and reads a large deterministic file through the world API.
func runWorldLargeUnixFSUpload(ctx context.Context, c *config) error {
	// Commit an empty UnixFS root in a world on the volume.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	le := logrus.NewEntry(logrus.New())
	ws, releaseWorld, err := initUnixFSWorld(ctx, le, c, vol, vol)
	if err != nil {
		return err
	}
	defer releaseWorld()

	// Write the large file through a batch writer.
	totalSize := largeFileSize(c)
	b := unixfs_world.NewBatchFSWriter(
		ws,
		"files",
		unixfs_world.FSType_FSType_FS_NODE,
		vol.GetPeerID(),
	)
	defer b.Release()
	if err := b.AddFile(
		ctx,
		nil,
		"large-video.mp4",
		unixfs_sdk.NewFSCursorNodeType_File(),
		int64(totalSize),
		newDeterministicLargeReader(totalSize, 0),
		0o644,
		time.Now(),
	); err != nil {
		return errors.Wrap(err, "add large unixfs file")
	}
	if err := b.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit large unixfs upload")
	}

	// Read the file back through a handle on the root.
	handle, _, releaseHandle, err := openUnixFSHandle(ctx, le, ws, vol.GetPeerID())
	if err != nil {
		return err
	}
	defer releaseHandle()
	largeFile, err := handle.Lookup(ctx, "large-video.mp4")
	if err != nil {
		return errors.Wrap(err, "lookup large file")
	}
	defer largeFile.Release()
	return verifyDeterministicFSFile(ctx, largeFile, totalSize, 0, c)
}

// runWorldResourceLargeUnixFSUpload checks the resource upload contract over a direct volume.
func runWorldResourceLargeUnixFSUpload(ctx context.Context, c *config) error {
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	return runWorldResourceLargeUnixFSUploadOnBucket(ctx, c, vol, vol)
}

// runWorldResourceDirectUploadTreeLargeUnixFSUpload checks UploadTree without the RPC transport.
func runWorldResourceDirectUploadTreeLargeUnixFSUpload(ctx context.Context, c *config) error {
	// Commit an empty UnixFS root in a world on the volume.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	le := logrus.NewEntry(logrus.New())
	ws, releaseWorld, err := initUnixFSWorld(ctx, le, c, vol, vol)
	if err != nil {
		return err
	}
	defer releaseWorld()

	// Serve the root as a UnixFS resource.
	handle, fsCursor, releaseHandle, err := openUnixFSHandle(ctx, le, ws, vol.GetPeerID())
	if err != nil {
		return err
	}
	defer releaseHandle()
	rootResource := resource_unixfs.NewFSHandleObjectResource(
		logrus.NewEntry(logrus.StandardLogger()),
		handle,
		fsCursor,
		nil,
		ws,
		"files",
		unixfs_world.FSType_FSType_FS_NODE,
		nil,
	)

	// Upload the generated tree directly and check the written counts.
	totalSize := largeFileSize(c)
	postProgress(c, "direct-upload-tree-start", 0, totalSize)
	resp, err := rootResource.UploadTree(newGeneratedUploadTreeStream(ctx, c, "large-video.mp4", totalSize, 0))
	if err != nil {
		return errors.Wrap(err, "direct UploadTree")
	}
	written := resp.GetWritten()
	postProgress(c, "direct-upload-tree-complete", int(written.GetBytesWritten()), totalSize)
	if written.GetBytesWritten() != int64(totalSize) {
		return errors.Errorf("UploadTree bytes_written=%d want=%d", written.GetBytesWritten(), totalSize)
	}
	if written.GetFilesWritten() != 1 {
		return errors.Errorf("UploadTree files_written=%d want=1", written.GetFilesWritten())
	}

	// Read the file back through the resource's handle.
	largeFile, err := handle.Lookup(ctx, "large-video.mp4")
	if err != nil {
		return errors.Wrap(err, "lookup direct uploaded file")
	}
	defer largeFile.Release()
	return verifyDeterministicFSFile(ctx, largeFile, totalSize, 0, c)
}

// runWorldControllerResourceLargeUnixFSUpload checks resource upload through a live volume controller.
func runWorldControllerResourceLargeUnixFSUpload(ctx context.Context, c *config) (retErr error) {
	vol, bkt, cleanup, err := openControllerBucket(ctx, c)
	if err != nil {
		return err
	}
	defer func() {
		if err := cleanup(); retErr == nil && err != nil {
			retErr = err
		}
	}()

	return runWorldResourceLargeUnixFSUploadOnBucket(ctx, c, vol, bkt)
}

// runWorldCloudOverlayResourceLargeUnixFSUpload checks resource upload through the dirty-tracking overlay.
func runWorldCloudOverlayResourceLargeUnixFSUpload(ctx context.Context, c *config, flush bool) (retErr error) {
	vol, bkt, cleanup, err := openControllerCloudOverlayBucket(ctx, c, flush)
	if err != nil {
		return err
	}
	defer func() {
		if err := cleanup(); retErr == nil && err != nil {
			retErr = err
		}
	}()

	return runWorldResourceLargeUnixFSUploadOnBucket(ctx, c, vol, bkt)
}

// runWorldResourceLargeUnixFSUploadOnBucket checks streamed upload and readback through resource clients.
func runWorldResourceLargeUnixFSUploadOnBucket(
	ctx context.Context,
	c *config,
	vol volume.Volume,
	bkt bucket.BucketOps,
) (retErr error) {
	// Commit an empty UnixFS root in a world on the bucket.
	le := logrus.NewEntry(logrus.New())
	ws, releaseWorld, err := initUnixFSWorld(ctx, le, c, vol, bkt)
	if err != nil {
		return err
	}
	defer releaseWorld()

	// Serve the root as a UnixFS resource through a resource client.
	handle, fsCursor, releaseHandle, err := openUnixFSHandle(ctx, le, ws, vol.GetPeerID())
	if err != nil {
		return err
	}
	defer releaseHandle()
	rootResource := resource_unixfs.NewFSHandleObjectResource(
		logrus.NewEntry(logrus.StandardLogger()),
		handle,
		fsCursor,
		nil,
		ws,
		"files",
		unixfs_world.FSType_FSType_FS_NODE,
		nil,
	)
	resClient, cleanup, err := openResourceClient(ctx, rootResource.GetMux())
	if err != nil {
		return err
	}
	defer func() {
		if err := cleanup(); retErr == nil && err != nil {
			retErr = err
		}
	}()

	// Open the root resource's service.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		return errors.Wrap(err, "get root resource client")
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// Upload the file, stopping there for the write-only scenario.
	totalSize := largeFileSize(c)
	postProgress(c, "resource-upload-start", 0, totalSize)
	if err := uploadDeterministicResourceFile(ctx, rootSvc, "large-video.mp4", totalSize, 0, c); err != nil {
		return err
	}
	postProgress(c, "resource-upload-complete", totalSize, totalSize)
	if c.scenario == "world-resource-large-unixfs-write" {
		return nil
	}

	// Look up the uploaded file as a new resource.
	postProgress(c, "resource-lookup-start")
	fileResp, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "large-video.mp4",
	})
	if err != nil {
		return errors.Wrap(err, "lookup uploaded resource file")
	}
	postProgress(c, "resource-lookup-complete")
	fileRef := resClient.CreateResourceReference(fileResp.GetResourceId())
	defer fileRef.Release()

	// Open the file resource's client.
	postProgress(c, "resource-client-start")
	fileClient, err := fileRef.GetClient()
	if err != nil {
		return errors.Wrap(err, "get uploaded resource file client")
	}
	postProgress(c, "resource-client-complete")

	// Read the file back through its resource.
	fileSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(fileClient)
	postProgress(c, "resource-readback-start", 0, totalSize)
	if err := verifyDeterministicResourceFile(ctx, fileSvc, totalSize, 0, c); err != nil {
		return err
	}
	postProgress(c, "resource-readback-complete", totalSize, totalSize)
	return nil
}

// openControllerBucket returns a running volume controller's bucket and ordered cleanup.
func openControllerBucket(
	ctx context.Context,
	c *config,
) (volume.Volume, bucket.BucketOps, func() error, error) {
	// Run a volume controller on the browser volume.
	le := logrus.NewEntry(logrus.New())
	ctrlCtx, cancelCtrl := context.WithCancel(ctx)
	ctrl := volume_controller.NewController(
		le,
		&volume_controller.Config{DisablePeer: true},
		nil,
		controller.NewInfo(
			volume_browser.ControllerID,
			volume_browser.Version,
			"opfs-chrometest@"+c.root,
		),
		func(ctx context.Context, _ *logrus.Entry) (volume.Volume, error) {
			return volume_browser.NewVolume(ctx, newVolumeConfig(c))
		},
	)
	ctrlErrCh := make(chan error, 1)
	go func() {
		ctrlErrCh <- ctrl.Execute(ctrlCtx)
	}()

	// Stop the controller on cleanup, ignoring the cancellation itself.
	cleanup := func() error {
		cancelCtrl()
		err := <-ctrlErrCh
		if err != nil && !stderrors.Is(err, context.Canceled) {
			return err
		}
		return nil
	}

	// Wait for the volume and apply the world bucket's config.
	vol, err := ctrl.GetVolume(ctx)
	if err != nil {
		_ = cleanup()
		return nil, nil, nil, err
	}
	bucketID := c.root + "/world"
	if _, _, _, err := vol.ApplyBucketConfig(ctx, &bucket.Config{
		Id:  bucketID,
		Rev: 1,
	}); err != nil {
		_ = cleanup()
		return nil, nil, nil, errors.Wrap(err, "apply controller bucket config")
	}

	// Build the bucket API, releasing it before the controller on cleanup.
	bktHandle, releaseBucket, err := ctrl.BuildBucketAPI(ctx, bucketID)
	if err != nil {
		_ = cleanup()
		return nil, nil, nil, errors.Wrap(err, "build controller bucket api")
	}
	bkt := bktHandle.GetBucket()
	if bkt == nil {
		releaseBucket()
		_ = cleanup()
		return nil, nil, nil, errors.New("controller bucket handle did not exist")
	}
	return vol, bkt, func() error {
		releaseBucket()
		return cleanup()
	}, nil
}

// runCopyWalkWrapperConcurrency probes whether the production
// AccessWorldState -> FollowRef -> lookup Handle -> CopyObjectToBucket ->
// WalkObjectBlocks wrapper deadlocks at a raised maxConcurrency on real OPFS
// under native Go-WASM, with the GoScript compiler held out of the loop. A
// prior bench drove the raw OPFS engine at concurrency 16 directly, and the
// engine stayed healthy. runCopyWalkWrapperConcurrency exercises the full
// wrapper path the production download-manifest copy uses, including the real
// concurrent-lookup Handle that resolves the cross-bucket source ref.
//
// It stands up a real controllerbus over the OPFS volume with the
// concurrent-lookup controller (so cross-bucket FollowRef resolves through the
// production Handle), builds a wide source-object block DAG in a bucket distinct
// from the dest world root (CopyObjectToBucket no-ops when src and dest share a
// bucket), then runs the production nested-access copy pattern twice over fresh
// equivalent source objects: first at maxConcurrency=1 (control, must pass),
// then at c.batch (the suspect, default 16). c.iterations is the source input
// byte count. The JC chunker fans it into hundreds of leaf blocks.
func runCopyWalkWrapperConcurrency(ctx context.Context, c *config) error {
	// Default the source size and the suspect concurrency.
	inputBytes := c.iterations
	if inputBytes <= 0 {
		inputBytes = 64 * 1024
	}
	suspectConc := c.batch
	if suspectConc <= 0 {
		suspectConc = 16
	}

	// Build a real bus over the browser volume so cross-bucket FollowRef
	// resolves through the production concurrent-lookup Handle, matching
	// download-manifest, and run the configset controller on it.
	le := logrus.NewEntry(logrus.New())
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return errors.Wrap(err, "construct core bus")
	}
	sr.AddFactory(volume_browser.NewFactory(b))
	_, _, csRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&configset_controller.Config{}),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "load configset controller")
	}
	defer csRef.Release()

	// Node controller owns per-bucket lookup loading: it reacts to applied
	// bucket configs and loads the concurrent-lookup controller that resolves
	// BuildBucketLookup. Without it FollowRef waits forever for the lookup Handle.
	_, _, nodeRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&node_controller.Config{}),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "load node controller")
	}
	defer nodeRef.Release()

	// Run the browser volume controller and wait for its volume.
	volDV, _, volRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(newVolumeConfig(c)),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "load browser volume controller")
	}
	defer volRef.Release()
	vol, err := volDV.(volume.Controller).GetVolume(ctx)
	if err != nil {
		return errors.Wrap(err, "get browser volume")
	}
	volID := vol.GetID()

	// Bucket config carrying the concurrent-lookup controller so each bucket
	// resolves through the same Handle the production world path uses. Single
	// node, so the default NONE not-found behavior (no remote lookup wait).
	lookupConf := node_controller.BuildDefaultLookupConfig()
	lookupCC, err := csp.NewControllerConfig(configset.NewControllerConfig(1, lookupConf), false)
	if err != nil {
		return errors.Wrap(err, "encode lookup controller config")
	}

	// Apply the world and source bucket configs.
	worldBucketID := c.root + "/world"
	sourceBucketID := c.root + "/source"
	for _, bucketID := range []string{worldBucketID, sourceBucketID} {
		if _, err := bucket.ExApplyBucketConfig(ctx, b, bucket.NewApplyBucketConfigToVolume(
			&bucket.Config{
				Id:     bucketID,
				Rev:    1,
				Lookup: &bucket.LookupConfig{Controller: lookupCC},
			},
			volID,
		)); err != nil {
			return errors.Wrapf(err, "apply bucket config %s", bucketID)
		}
	}

	// Share a gzip transform so stored bytes hash consistently with their
	// object refs across both buckets. CopyObjectToBucket's forced-ref
	// writes require the source stored representation to match its ref.
	sfs := transform_all.BuildFactorySet()
	transformConf, err := block_transform.NewConfig([]cbconfig.Config{
		&transform_gzip.Config{},
	})
	if err != nil {
		return errors.Wrap(err, "build transform config")
	}

	// Build the destination world state on the world bucket.
	worldCursor, _, err := bucket_lookup.BuildEmptyCursor(ctx, b, le, sfs, worldBucketID, volID, transformConf, nil)
	if err != nil {
		return errors.Wrap(err, "build world cursor")
	}
	defer worldCursor.Release()
	ws, err := world_block.BuildWorldStateFromCursor(
		ctx,
		le,
		true,
		worldCursor,
		world.NewWorldStorageFromCursor(worldCursor),
		lookupSpaceOp,
		false,
	)
	if err != nil {
		return errors.Wrap(err, "build world state")
	}
	defer ws.Discard()

	// Source cursor for building wide DAGs in a distinct bucket, bus-backed.
	sourceCursor, _, err := bucket_lookup.BuildEmptyCursor(ctx, b, le, sfs, sourceBucketID, volID, transformConf, nil)
	if err != nil {
		return errors.Wrap(err, "build source cursor")
	}
	defer sourceCursor.Release()
	sourceAccess := world.NewAccessWorldStateFunc(sourceCursor)

	// Build sources with small chunks for a wide DAG: ChunkIndex -> many Chunk
	// -> many ByteSlice leaves, so WalkObjectBlocks has real fan-out to
	// schedule.
	blobOpts := &blob.BuildBlobOpts{
		RawHighWaterMark: 1,
		ChunkerArgs: &blob.ChunkerArgs{
			ChunkerType: blob.ChunkerType_ChunkerType_JC,
			JcArgs: &blob.JcArgs{
				ChunkingMinSize:    64,
				ChunkingTargetSize: 128,
				ChunkingMaxSize:    256,
			},
		},
	}
	buildSource := func(salt int) (*bucket.ObjectRef, error) {
		return world.AccessObject(ctx, sourceAccess, nil, func(bcs *block.Cursor) error {
			_, err := blob.BuildBlob(
				ctx,
				int64(inputBytes),
				newDeterministicLargeReader(inputBytes, salt),
				bcs,
				blobOpts,
			)
			return err
		})
	}

	// Copy a fresh source at the control concurrency, then at the suspect one.
	runs := []struct {
		label string
		conc  int
	}{
		{label: "control", conc: 1},
		{label: "suspect", conc: suspectConc},
	}
	for i, r := range runs {
		postProgress(c, "copy-walk-source-build-start", i, r.conc)
		srcObjRef, err := buildSource(i + 1)
		if err != nil {
			return errors.Wrapf(err, "build source DAG (%s)", r.label)
		}
		postProgress(c, "copy-walk-source-build-complete", i, r.conc)

		postProgress(c, "copy-walk-copy-start", i, r.conc)
		if err := runCopyWalkWrapperCopy(ctx, le, ws, srcObjRef, r.conc, r.label); err != nil {
			return errors.Wrapf(err, "%s copy at concurrency %d", r.label, r.conc)
		}
		postProgress(c, "copy-walk-copy-complete", i, r.conc)
	}

	// Report the probe's dimensions.
	c.extra = map[string]int64{
		"inputBytes":  int64(inputBytes),
		"controlConc": 1,
		"suspectConc": int64(suspectConc),
	}
	return nil
}

// runCopyWalkWrapperCopy runs one wrapper copy mirroring the production
// nested-access shape: dest world bucket, then source bucket via cross-bucket
// FollowRef, then CopyObjectToBucket + Sync. A concurrency regression in the
// wrapper surfaces as the copy never returning, which the chrome harness context
// deadline turns into a test failure.
func runCopyWalkWrapperCopy(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	srcObjRef *bucket.ObjectRef,
	maxConcurrency int,
	label string,
) error {
	return ws.AccessWorldState(ctx, nil, func(dest *bucket_lookup.Cursor) error {
		return ws.AccessWorldState(ctx, srcObjRef, func(src *bucket_lookup.Cursor) error {
			le.Infof(
				"copy-walk-wrapper %s: copying DAG bucket %s -> %s at concurrency %d",
				label,
				src.GetOpArgs().GetBucketId(),
				dest.GetOpArgs().GetBucketId(),
				maxConcurrency,
			)
			if _, err := bucket_lookup.CopyObjectToBucket(
				ctx,
				dest,
				src,
				blob.NewBlobBlock,
				maxConcurrency,
				false,
				nil,
			); err != nil {
				return errors.Wrap(err, "copy object to bucket")
			}
			if _, err := ws.Sync(ctx); err != nil {
				return errors.Wrap(err, "sync copied blocks")
			}
			return nil
		})
	})
}

// openControllerCloudOverlayBucket wraps a controller bucket with dirty tracking and optional packing.
func openControllerCloudOverlayBucket(
	ctx context.Context,
	c *config,
	syncDuringUpload bool,
) (volume.Volume, bucket.BucketOps, func() error, error) {
	// Open the controller bucket and the dirty index store.
	vol, upper, cleanupBucket, err := openControllerBucket(ctx, c)
	if err != nil {
		return nil, nil, nil, err
	}
	objStore, releaseObjStore, err := vol.AccessObjectStore(ctx, c.root+"/cloud-overlay-meta", func() {})
	if err != nil {
		_ = cleanupBucket()
		return nil, nil, nil, errors.Wrap(err, "open cloud overlay dirty store")
	}

	// Wrap the bucket in a dirty-tracking write cache, packing as it goes when
	// requested.
	var flusher *probeSyncFlusher
	if syncDuringUpload {
		flusher = newProbeSyncFlusher(upper, objStore)
	}
	dirtyUpper := &probeDirtyTrackingStore{store: upper, dirtyStore: objStore, flusher: flusher}
	overlay := block.NewOverlay(
		ctx,
		logrus.NewEntry(logrus.New()),
		block.NopStoreOps{},
		dirtyUpper,
		block.OverlayMode_UPPER_WRITE_CACHE,
		0,
		nil,
	)

	// Wait for the packing, then release the store and the bucket.
	return vol, overlay, func() error {
		// Join the packer, then release in order, keeping the first error.
		var err error
		if flusher != nil {
			err = flusher.wait()
		}
		releaseObjStore()
		if cleanupErr := cleanupBucket(); err == nil {
			err = cleanupErr
		}
		return err
	}, nil
}

// uploadDeterministicResourceFile streams one deterministic file through UploadTree.
func uploadDeterministicResourceFile(
	ctx context.Context,
	rootSvc s4wave_unixfs.SRPCFSHandleResourceServiceClient,
	name string,
	totalSize int,
	salt int,
	c *config,
) error {
	// Open the stream and send the file header.
	strm, err := rootSvc.UploadTree(ctx)
	if err != nil {
		return errors.Wrap(err, "open UploadTree stream")
	}
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
			FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
				Path:      name,
				TotalSize: int64(totalSize),
				Mode:      0o644,
				Expect:    &s4wave_unixfs.HandleUploadTreeFileStart_Overwrite{Overwrite: true},
			},
		},
	}); err != nil {
		return errors.Wrap(err, "send UploadTree file_start")
	}

	// Stream the payload in 64 KiB chunks.
	const chunkSize = 64 * 1024
	for offset := 0; offset < totalSize; offset += chunkSize {
		n := min(chunkSize, totalSize-offset)
		if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
			Body: &s4wave_unixfs.HandleUploadTreeRequest_Data{
				Data: deterministicLargeWindow(offset, n, salt),
			},
		}); err != nil {
			return errors.Wrapf(err, "send UploadTree data offset=%d", offset)
		}
		next := offset + n
		if next == totalSize || next%largeScenarioProgressEvery == 0 {
			postProgress(c, "resource-upload-stream", next, totalSize)
		}
	}

	// Close the stream and check the written totals.
	postProgress(c, "resource-upload-close-start", totalSize, totalSize)
	resp, err := strm.CloseAndRecv()
	if err != nil {
		return errors.Wrap(err, "close UploadTree stream")
	}
	postProgress(c, "resource-upload-close-complete", totalSize, totalSize)
	written := resp.GetWritten()
	if written.GetBytesWritten() != int64(totalSize) {
		return errors.Errorf("UploadTree bytes_written=%d want=%d", written.GetBytesWritten(), totalSize)
	}
	if written.GetFilesWritten() != 1 {
		return errors.Errorf("UploadTree files_written=%d want=1", written.GetFilesWritten())
	}
	return nil
}

// verifyDeterministicResourceFile checks file size, sampled offsets, and full content over RPC.
func verifyDeterministicResourceFile(
	ctx context.Context,
	fileSvc s4wave_unixfs.SRPCFSHandleResourceServiceClient,
	totalSize int,
	salt int,
	c *config,
) error {
	// Check the size.
	postProgress(c, "resource-readback-size-start", 0, totalSize)
	sizeResp, err := fileSvc.GetSize(ctx, &s4wave_unixfs.HandleGetSizeRequest{})
	if err != nil {
		return errors.Wrap(err, "get uploaded resource file size")
	}
	postProgress(c, "resource-readback-size-complete", int(sizeResp.GetSize()), totalSize)
	if sizeResp.GetSize() != uint64(totalSize) {
		return errors.Errorf("resource file size=%d want=%d", sizeResp.GetSize(), totalSize)
	}

	// Check sampled windows at the start, middle, and end.
	for _, offset := range []int{0, 4096, totalSize / 2, max(0, totalSize-4096)} {
		wantLen := min(4096, totalSize-offset)
		postProgress(c, "resource-readback-read-start", offset, totalSize)
		resp, err := fileSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{
			Offset: int64(offset),
			Length: int64(wantLen),
		})
		if err != nil {
			return errors.Wrapf(err, "read uploaded resource file offset=%d", offset)
		}
		postProgress(c, "resource-readback-read-complete", offset+len(resp.GetData()), totalSize)
		if len(resp.GetData()) != wantLen {
			return errors.Errorf("resource file offset=%d read=%d want=%d", offset, len(resp.GetData()), wantLen)
		}
		want := deterministicLargeWindow(offset, wantLen, salt)
		if !bytes.Equal(resp.GetData(), want) {
			return errors.Errorf("resource file offset=%d data mismatch", offset)
		}
	}

	// Read back the whole file in bounded windows.
	postProgress(c, "resource-readback-full-start", 0, totalSize)
	fullReadChunkSize := resourceFullReadChunkSize(c)
	fullReadProgressEvery := max(largeScenarioProgressEvery, fullReadChunkSize)
	for offset := 0; offset < totalSize; {
		wantLen := min(fullReadChunkSize, totalSize-offset)
		resp, err := fileSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{
			Offset: int64(offset),
			Length: int64(wantLen),
		})
		if err != nil {
			return errors.Wrapf(err, "full read uploaded resource file offset=%d", offset)
		}

		// Check the returned bytes and advance.
		got := resp.GetData()
		if len(got) == 0 {
			return errors.Errorf("full read resource file offset=%d read=0 want progress", offset)
		}
		if len(got) > wantLen {
			return errors.Errorf("full read resource file offset=%d read=%d max=%d", offset, len(got), wantLen)
		}
		want := deterministicLargeWindow(offset, len(got), salt)
		if !bytes.Equal(got, want) {
			return errors.Errorf("full read resource file offset=%d data mismatch", offset)
		}
		offset += len(got)
		if offset == totalSize || offset%fullReadProgressEvery == 0 {
			postProgress(c, "resource-readback-full-stream", offset, totalSize)
		}
	}
	postProgress(c, "resource-readback-full-complete", totalSize, totalSize)
	return nil
}

// resourceFullReadChunkSize returns the requested readback window or its bounded default.
func resourceFullReadChunkSize(c *config) int {
	if c != nil && c.batch > 0 {
		return c.batch
	}
	return 256 * 1024
}

// verifyDeterministicFSFile checks file size, sampled offsets, and full content through FSHandle.
func verifyDeterministicFSFile(
	ctx context.Context,
	handle *unixfs_sdk.FSHandle,
	totalSize int,
	salt int,
	c *config,
) error {
	// Check the size.
	postProgress(c, "fs-readback-size-start", 0, totalSize)
	size, err := handle.GetSize(ctx)
	if err != nil {
		return errors.Wrap(err, "get large file size")
	}
	postProgress(c, "fs-readback-size-complete", int(size), totalSize)
	if size != uint64(totalSize) {
		return errors.Errorf("large file size=%d want=%d", size, totalSize)
	}

	// Check sampled windows at the start, middle, and end.
	for _, offset := range []int{0, 4096, totalSize / 2, max(0, totalSize-4096)} {
		wantLen := min(4096, totalSize-offset)
		got := make([]byte, wantLen)
		postProgress(c, "fs-readback-read-start", offset, totalSize)
		n, err := handle.ReadAt(ctx, int64(offset), got)
		if err != nil && err != io.EOF {
			return errors.Wrapf(err, "read large file offset=%d", offset)
		}
		postProgress(c, "fs-readback-read-complete", offset+int(n), totalSize)
		if int(n) != wantLen {
			return errors.Errorf("large file offset=%d read=%d want=%d", offset, n, wantLen)
		}
		want := deterministicLargeWindow(offset, wantLen, salt)
		if !bytes.Equal(got, want) {
			return errors.Errorf("large file offset=%d data mismatch", offset)
		}
	}

	// Read back the whole file in bounded windows.
	postProgress(c, "fs-readback-full-start", 0, totalSize)
	fullReadChunkSize := resourceFullReadChunkSize(c)
	fullReadProgressEvery := max(largeScenarioProgressEvery, fullReadChunkSize)
	for offset := 0; offset < totalSize; {
		wantLen := min(fullReadChunkSize, totalSize-offset)
		got := make([]byte, wantLen)
		n, err := handle.ReadAt(ctx, int64(offset), got)
		if err != nil && err != io.EOF {
			return errors.Wrapf(err, "full read large file offset=%d", offset)
		}
		if n <= 0 {
			return errors.Errorf("full read large file offset=%d read=0 want progress", offset)
		}

		// Check the returned bytes and advance.
		got = got[:int(n)]
		want := deterministicLargeWindow(offset, len(got), salt)
		if !bytes.Equal(got, want) {
			return errors.Errorf("full read large file offset=%d data mismatch", offset)
		}
		offset += len(got)
		if offset == totalSize || offset%fullReadProgressEvery == 0 {
			postProgress(c, "fs-readback-full-stream", offset, totalSize)
		}
	}
	postProgress(c, "fs-readback-full-complete", totalSize, totalSize)
	return nil
}

// generatedUploadTreeStream generates one file stream without retaining its full payload.
type generatedUploadTreeStream struct {
	// ctx bounds the upload lifetime.
	ctx context.Context
	// c identifies the worker for progress reports.
	c *config
	// name holds the uploaded file's name.
	name string
	// totalSize is the complete file length.
	totalSize int
	// salt selects the reproducible payload.
	salt int
	// offset is the next unread byte offset.
	offset int
	// startSent records whether the file header has been emitted.
	startSent bool
}

// newGeneratedUploadTreeStream constructs a deterministic UploadTree request source.
func newGeneratedUploadTreeStream(
	ctx context.Context,
	c *config,
	name string,
	totalSize int,
	salt int,
) *generatedUploadTreeStream {
	return &generatedUploadTreeStream{
		ctx:       ctx,
		c:         c,
		name:      name,
		totalSize: totalSize,
		salt:      salt,
	}
}

// Context returns the upload operation's cancellation context.
func (s *generatedUploadTreeStream) Context() context.Context {
	return s.ctx
}

// MsgSend accepts the unused response direction of the local upload stream.
func (s *generatedUploadTreeStream) MsgSend(srpc.Message) error {
	return nil
}

// MsgRecv fills a typed upload request from the next generated message.
func (s *generatedUploadTreeStream) MsgRecv(msg srpc.Message) error {
	req, ok := msg.(*s4wave_unixfs.HandleUploadTreeRequest)
	if !ok {
		return errors.Errorf("unexpected UploadTree stream recv target %T", msg)
	}
	return s.RecvTo(req)
}

// CloseSend accepts closure of the unused response direction.
func (s *generatedUploadTreeStream) CloseSend() error {
	return nil
}

// Close accepts closure of the generated stream, which owns no external handles.
func (s *generatedUploadTreeStream) Close() error {
	return nil
}

// Recv emits the file header, bounded data chunks, and then EOF.
func (s *generatedUploadTreeStream) Recv() (*s4wave_unixfs.HandleUploadTreeRequest, error) {
	// Emit the header first.
	if !s.startSent {
		s.startSent = true
		postProgress(s.c, "direct-upload-tree-file-start", 0, s.totalSize)
		return &s4wave_unixfs.HandleUploadTreeRequest{
			Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
				FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
					Path:      s.name,
					TotalSize: int64(s.totalSize),
					Mode:      0o644,
					Expect:    &s4wave_unixfs.HandleUploadTreeFileStart_Overwrite{Overwrite: true},
				},
			},
		}, nil
	}

	// Emit EOF after the last chunk.
	if s.offset >= s.totalSize {
		postProgress(s.c, "direct-upload-tree-eof", s.totalSize, s.totalSize)
		return nil, io.EOF
	}

	// Emit the next 64 KiB chunk, reporting every 8 MiB.
	const chunkSize = 64 * 1024
	const progressEvery = 8 * 1024 * 1024
	n := min(chunkSize, s.totalSize-s.offset)
	offset := s.offset
	s.offset += n
	if s.offset == s.totalSize || s.offset%progressEvery == 0 {
		postProgress(s.c, "direct-upload-tree-stream", s.offset, s.totalSize)
	}
	return &s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_Data{
			Data: deterministicLargeWindow(offset, n, s.salt),
		},
	}, nil
}

// RecvTo copies the next generated message into the caller's request.
func (s *generatedUploadTreeStream) RecvTo(req *s4wave_unixfs.HandleUploadTreeRequest) error {
	next, err := s.Recv()
	if err != nil {
		return err
	}
	*req = *next
	return nil
}
