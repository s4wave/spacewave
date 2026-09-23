package resource_space

import (
	"context"
	"io/fs"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	uuid "github.com/satori/go.uuid"
)

// BuildSpacePlugin queues an immutable source snapshot through the existing Forge owner.
// The mounted session supplies the sender; the selected Device supplies execution identity.
func (r *SpaceResource) BuildSpacePlugin(ctx context.Context, req *s4wave_space.BuildSpacePluginRequest) (*s4wave_space.BuildSpacePluginResponse, error) {
	queued, err := r.queuePluginBuild(ctx, req, &space_exec.PluginBuildConfig{})
	if err != nil {
		return nil, err
	}
	return &s4wave_space.BuildSpacePluginResponse{ExecutionKey: queued.key}, nil
}

// queuedPluginBuild identifies the accepted execution and assigned device Session.
type queuedPluginBuild struct {
	// key is the durable Forge execution object key.
	key string
	// peer is the device's authenticated transport identity.
	peer peer.ID
}

// queuePluginBuild captures source and device authority in one World transaction.
func (r *SpaceResource) queuePluginBuild(ctx context.Context, req *s4wave_space.BuildSpacePluginRequest, config *space_exec.PluginBuildConfig) (*queuedPluginBuild, error) {
	if r.sessionPeerID == "" {
		return nil, errors.New("plugin builds require a mounted session identity")
	}
	sender, err := confparse.ParsePeerID(r.sessionPeerID)
	if err != nil {
		return nil, err
	}
	if err := manifest.ValidateManifestID(req.GetManifestId(), false); err != nil {
		return nil, err
	}
	configPath := req.GetConfigPath()
	if configPath == "" {
		configPath = "bldr.yaml"
	}
	if !fs.ValidPath(configPath) {
		return nil, errors.New("config_path must be relative to the source tree")
	}

	// Select the device and source from the same accepted World transaction.
	tx, err := r.space.GetWorldEngine().NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	deviceType, err := world_types.GetObjectType(ctx, tx, req.GetDeviceKey())
	if err != nil {
		return nil, err
	}
	if deviceType != s4wave_device.DeviceTypeID {
		return nil, errors.New("select a registered build device")
	}
	device, object, err := world.LookupObject[*s4wave_device.Device](ctx, tx, req.GetDeviceKey(), s4wave_device.NewDeviceBlock)
	world.ReleaseObjectState(object)
	if err != nil {
		return nil, err
	}
	worker := device.FindSelectableForgeWorker()
	if !device.IsSelectable() || worker == nil {
		return nil, errors.New("device has no available Forge worker")
	}
	workerKey := worker.GetLink().GetObjectKey()
	if err := forge_worker.CheckWorkerType(ctx, tx, workerKey); err != nil {
		return nil, err
	}
	devicePeer, err := confparse.ParsePeerID(device.GetPeerId())
	if err != nil {
		return nil, err
	}
	keypairs, _, err := forge_worker.CollectWorkerKeypairs(ctx, tx, workerKey)
	if err != nil {
		return nil, err
	}
	assigned := false
	for _, keypair := range keypairs {
		if keypair.GetPeerId() == devicePeer.String() {
			assigned = true
			break
		}
	}
	if !assigned {
		return nil, errors.New("device session does not own its selected Forge worker")
	}
	sourceObject, err := world.MustGetObject(ctx, tx, req.GetSourceKey())
	if err != nil {
		return nil, err
	}
	source, err := forge_value.NewWorldObjectSnapshot(ctx, sourceObject, tx)
	world.ReleaseObjectState(sourceObject)
	if err != nil {
		return nil, err
	}
	if source.GetObjectType() != unixfs_world.FSNodeTypeID {
		return nil, errors.New("plugin source must be a Space UnixFS directory")
	}

	// The Execution owns the input DAG and survives the submitting client's lifetime.
	key := "plugin-builds/" + uuid.NewV4().String()
	config.ManifestId = req.GetManifestId()
	config.ConfigPath = configPath
	if config.GetFrontendId() != "" {
		config.FrontendPeerId = sender.String()
	}
	configData, err := config.MarshalJSON()
	if err != nil {
		return nil, err
	}
	_, err = forge_execution.CreateExecutionWithTarget(ctx, tx, sender, key, devicePeer,
		&forge_target.ValueSet{
			Inputs: forge_value.ValueSlice{forge_value.NewValueWithWorldObjectSnapshot("source", source)},
		}, &forge_target.Target{
			Exec: &forge_target.Exec{
				Controller: &configset_proto.ControllerConfig{Id: space_exec.BuildPluginConfigID, Rev: 1, Config: configData},
			},
		}, timestamppb.Now())
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &queuedPluginBuild{key: key, peer: devicePeer}, nil
}
