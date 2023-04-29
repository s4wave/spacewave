package bldr_dist_compiler

import (
	"context"
	io "io"
	"os"
	"path"
	"strings"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bldr"
	bldr_dist "github.com/aperturerobotics/bldr/dist"
	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	"github.com/aperturerobotics/bldr/util/gocompiler"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	kvfile_compress "github.com/aperturerobotics/go-kvfile/compress"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	"github.com/aperturerobotics/hydra/bucket"
	hydra_core "github.com/aperturerobotics/hydra/core"
	kvtx_kvfile "github.com/aperturerobotics/hydra/kvtx/kvfile"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	volume_bolt "github.com/aperturerobotics/hydra/volume/bolt"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	"github.com/aperturerobotics/hydra/world"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// BuildDistBundle builds the distribution bundle for an application.
//
// initEmbeddedWorld should initialize the embedded manifest world.
func BuildDistBundle(
	rctx context.Context,
	le *logrus.Entry,
	workingPath, outputPath string,
	outBinName string,
	meta *bldr_dist.DistMeta,
	buildType bldr_manifest.BuildType,
	buildPlatform bldr_platform.Platform,
	hostConfigSet map[string]*configset_proto.ControllerConfig,
	initEmbeddedWorld func(ctx context.Context, embedEngine world.Engine, embedOpPeerID peer.ID) error,
	enableCgo bool,
) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// Write the bldr license file.
	bldrLicense := bldr.GetLicense()
	if err := os.WriteFile(path.Join(workingPath, "LICENSE"), []byte(bldrLicense), 0644); err != nil {
		return err
	}

	// NOTE: we use the go.mod from the parent program.
	// we compile under ${parent_program}/.bldr/build/...
	// the Go compiler will find the go.mod with reference to bldr in a parent dir

	// encode config set for embedded config set binary
	var hostConfigSetBin []byte
	if len(hostConfigSet) != 0 {
		configSetObj := &configset_proto.ConfigSet{
			Configurations: hostConfigSet,
		}
		var err error
		hostConfigSetBin, err = configSetObj.MarshalVT()
		if err != nil {
			return err
		}
	}

	// EntrypointBuildDir is the directory we will run "go build"
	entrypointBuildDir := path.Join(workingPath, "entrypoint")
	if err := os.MkdirAll(entrypointBuildDir, 0755); err != nil {
		return err
	}

	// Write the configset bin file.
	outConfigSetPath := path.Join(entrypointBuildDir, "config-set.bin")
	if err := os.WriteFile(outConfigSetPath, hostConfigSetBin, 0644); err != nil {
		return err
	}

	// construct a new bus to hold our working volume
	le.Info("initializing embedded volume")
	workBus, workSr, err := hydra_core.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}
	workSr.AddFactory(volume_bolt.NewFactory(workBus))
	workSr.AddFactory(world_block_engine.NewFactory(workBus))

	// run the node controller
	_, _, nref, err := loader.WaitExecControllerRunning(
		ctx,
		workBus,
		resolver.NewLoadControllerWithConfig(
			&node_controller.Config{},
		),
		nil,
	)
	if err != nil {
		return err
	}
	defer nref.Release()

	// workingID is a unique working id to use
	// used to derive some at-rest crypto keys
	// may be replaced with something w/ more randomness later
	workingID := strings.Join([]string{ControllerID, meta.GetProjectId(), buildPlatform.GetPlatformID()}, "/")

	// start with a boltdb on-disk in the working dir
	workingDb := path.Join(workingPath, "assets.db")
	workingDbVolID := "dist-working-vol"
	workingDbConf := &volume_bolt.Config{
		Path:       workingDb,
		NoWriteKey: true,
		VolumeConfig: &volume_controller.Config{
			VolumeIdAlias:           []string{workingDbVolID},
			DisableReconcilerQueues: true,
		},
	}
	workingVolCtrli, _, workingVolRef, err := loader.WaitExecControllerRunning(
		ctx,
		workBus,
		resolver.NewLoadControllerWithConfig(workingDbConf),
		nil,
	)
	if err != nil {
		return err
	}
	defer workingVolRef.Release()
	_ = workingVolCtrli
	workingVolCtrl, ok := workingVolCtrli.(*volume_controller.Controller)
	if !ok {
		return errors.New("unexpected type for volume controller")
	}
	workingVol, err := workingVolCtrl.GetVolume(ctx)
	if err != nil {
		return err
	}
	// We know that it's a bolt volume.
	boltVol, ok := workingVol.(*volume_bolt.Bolt)
	if !ok {
		return errors.New("unexpected type for volume")
	}

	// workingVol will be embedded in the dist binary & available to application.
	// it will contain the embedded manifests.

	// create the embedded manifests world
	embedWorldID := strings.Join([]string{"dist", meta.GetProjectId()}, "/")
	embedBucketID := embedWorldID
	embedObjStoreID := embedWorldID
	// TODO: do not replicate flag?
	embedXfrmConf, err := block_transform.NewConfig(buildEmbedTransformConf(workingID))
	if err != nil {
		return err
	}
	bucketConf, err := bucket.NewConfig(embedBucketID, 1, nil, &bucket.LookupConfig{
		// Disable: true,
	})
	if err != nil {
		return err
	}
	_, err = bucket.ExApplyBucketConfig(ctx, workBus, bucket.NewApplyBucketConfig(bucketConf, nil, []string{workingDbVolID}))
	if err != nil {
		return err
	}
	embedEngineConf := world_block_engine.NewConfig(
		embedWorldID,
		workingDbVolID,
		embedBucketID,
		embedObjStoreID,
		&bucket.ObjectRef{TransformConf: embedXfrmConf.CloneVT()},
		nil,
	)
	embedEngineCtrli, _, embedEngineCtrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		workBus,
		resolver.NewLoadControllerWithConfig(embedEngineConf),
		nil,
	)
	if err != nil {
		return err
	}
	defer embedEngineCtrlRef.Release()
	embedEngineCtrl, ok := embedEngineCtrli.(*world_block_engine.Controller)
	if !ok {
		return errors.New("unexpected type for world block engine controller")
	}
	embedEngine, err := embedEngineCtrl.GetWorldEngine(ctx)
	if err != nil {
		return err
	}
	_ = embedEngine

	// Write contents to the embedded world.
	le.Debug("copying contents to embedded volume")
	if err := initEmbeddedWorld(ctx, embedEngine, workingVol.GetPeerID()); err != nil {
		return err
	}

	// Close the embedded world controller, no longer needed.
	embedEngineCtrlRef.Release()

	// Build a seekable-zstd compressed kvfile with the embedded volume contents.
	le.Debug("packing embedded volume to seekable-zstd kvfile")
	embeddedVolumePath := path.Join(entrypointBuildDir, "volume.kvfile")
	embeddedVolFile, err := os.OpenFile(embeddedVolumePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	// Access the workingVol kvtx
	workingVolKvtx := boltVol.GetKvtxStore()

	// Write the kvfile
	err = kvfile_compress.UseCompressedWriter(embeddedVolFile, func(w io.Writer) error {
		return kvtx_kvfile.KvfileFromStore(w, workingVolKvtx)
	})
	if err != nil {
		_ = embeddedVolFile.Close()
		return err
	}
	if err := embeddedVolFile.Close(); err != nil {
		return err
	}

	// Format and write the main.go file.
	// TODO: process the kvfile in the dist entrypoint
	le.Debug("compiling dist entrypoint")
	entrypointSrc := FormatDistEntrypoint(meta)
	entrypointMainPath := path.Join(entrypointBuildDir, "main.go")
	if err := os.WriteFile(entrypointMainPath, []byte(entrypointSrc), 0644); err != nil {
		return err
	}

	outBinPath := path.Join(outputPath, outBinName)
	return gocompiler.ExecBuildEntrypoint(le, buildPlatform, workingPath, outBinPath, enableCgo)
}
