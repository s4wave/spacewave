package resource_space

import (
	"context"
	"io/fs"
	"slices"
	"strings"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	uuid "github.com/satori/go.uuid"
)

// pluginBuildTaskName names the single Task of a plugin build Job.
const pluginBuildTaskName = "build"

// BuildSpacePlugin queues a plugin build as a Forge Job in the Space World.
// The mounted session supplies the sender; the selected Device supplies the
// Worker, peer, and native platform. Forge owns the Job after the commit, and a
// successful build installs its artifact in the Space.
func (r *SpaceResource) BuildSpacePlugin(ctx context.Context, req *s4wave_space.BuildSpacePluginRequest) (*s4wave_space.BuildSpacePluginResponse, error) {
	// Open the transaction that commits the Job and its checks together.
	tx, err := r.space.GetWorldEngine().NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Resolve the authorized Device, the pinned source, and the build platform.
	selection, err := r.selectPluginBuild(ctx, tx, req)
	if err != nil {
		return nil, err
	}
	platformID, err := selection.buildPlatform(req.GetPlatformId())
	if err != nil {
		return nil, err
	}

	// Select the Cluster and check the Worker can hold the capacity request.
	workerKey := selection.placement.GetWorkerObjectKey()
	clusterKey, err := selectWorkerCluster(ctx, tx, workerKey, req.GetClusterKey())
	if err != nil {
		return nil, err
	}
	if err := checkPluginBuildCapacity(ctx, tx, workerKey, req.GetMilliCpu(), req.GetMemoryBytes()); err != nil {
		return nil, err
	}

	// The Task retains the source snapshot and build request for the Worker.
	// The mounted Session holds the authority to install plugins in this Space,
	// so a successful build installs as that Session.
	configData, err := (&space_exec.PluginBuildConfig{
		ManifestId:      req.GetManifestId(),
		ConfigPath:      selection.configPath,
		PlatformId:      platformID,
		MilliCpu:        req.GetMilliCpu(),
		MemoryBytes:     req.GetMemoryBytes(),
		InstallerPeerId: selection.sender.String(),
	}).MarshalJSON()
	if err != nil {
		return nil, err
	}
	target := &forge_target.Target{
		Inputs: []*forge_target.Input{{
			Name:      "source",
			InputType: forge_target.InputType_InputType_VALUE,
			Value:     forge_value.NewValueWithWorldObjectSnapshot("source", selection.source),
		}},
		Exec: &forge_target.Exec{
			Controller: &configset_proto.ControllerConfig{Id: space_exec.BuildPluginConfigID, Rev: 1, Config: configData},
		},
		Outputs: []*forge_target.Output{
			{Name: "source", OutputType: forge_target.OutputType_OutputType_EXEC, ExecOutput: "source"},
			{Name: "manifest", OutputType: forge_target.OutputType_OutputType_EXEC, ExecOutput: "manifest"},
			{Name: "build-result", OutputType: forge_target.OutputType_OutputType_EXEC, ExecOutput: "build-result"},
		},
	}

	// Create the Job on the selected Worker and assign it to the Cluster, which
	// assigns its Task to the Cluster's peer.
	jobKey := "plugin-builds/" + uuid.NewV4().String()
	tasks := map[string]*forge_target.Target{pluginBuildTaskName: target}
	job, _, err := forge_job.CreateJobWithTasks(ctx, tx, selection.sender, jobKey, tasks, "", selection.placement, timestamppb.Now())
	world.ReleaseObjectState(job)
	if err != nil {
		return nil, err
	}
	if _, _, err := forge_cluster.AssignJobToCluster(ctx, tx, clusterKey, jobKey, selection.sender); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &s4wave_space.BuildSpacePluginResponse{
		JobKey:  jobKey,
		TaskKey: forge_job.NewJobTaskKey(jobKey, pluginBuildTaskName),
	}, nil
}

// devicePlacement is the mounted Session and the Device Worker that runs its Job.
type devicePlacement struct {
	// sender is the mounted Session that submitted the Job.
	sender peer.ID
	// device is the selected Device.
	device *s4wave_device.Device
	// devicePeer is the Device's authenticated transport identity.
	devicePeer peer.ID
	// placement binds the Job to the Device's Forge Worker.
	placement *forge_worker.Placement
}

// selectDevicePlacement binds a Job from the mounted Session to the Forge
// Worker of the Device at deviceKey.
func (r *SpaceResource) selectDevicePlacement(ctx context.Context, tx world.Tx, deviceKey string) (*devicePlacement, error) {
	// Require the mounted Session as the sender.
	if r.sessionPeerID == "" {
		return nil, errors.New("forge jobs require a mounted session identity")
	}
	sender, err := confparse.ParsePeerID(r.sessionPeerID)
	if err != nil {
		return nil, err
	}

	// Select the Device with a linked Worker that the Session may place work on.
	device, err := lookupDevice(ctx, tx, deviceKey)
	if err != nil {
		return nil, err
	}
	worker := device.FindSelectableForgeWorker()
	if !device.IsSelectable() || worker == nil {
		return nil, errors.New("device has no available Forge worker")
	}

	// Bind the Job to the Device's peer and linked Worker.
	devicePeer, err := confparse.ParsePeerID(device.GetPeerId())
	if err != nil {
		return nil, err
	}
	placement := &forge_worker.Placement{WorkerObjectKey: worker.GetLink().GetObjectKey(), PeerId: devicePeer.String()}
	if err := placement.ValidateLinked(ctx, tx); err != nil {
		return nil, err
	}
	return &devicePlacement{sender: sender, device: device, devicePeer: devicePeer, placement: placement}, nil
}

// pluginBuildSelection is the authorized Device and pinned source of one build.
type pluginBuildSelection struct {
	devicePlacement
	// source is the exact Space UnixFS snapshot to build.
	source *forge_value.WorldObjectSnapshot
	// configPath is the Bldr project configuration path inside source.
	configPath string
}

// selectPluginBuild captures source and Device authority in one World transaction.
func (r *SpaceResource) selectPluginBuild(ctx context.Context, tx world.Tx, req *s4wave_space.BuildSpacePluginRequest) (*pluginBuildSelection, error) {
	// Validate the plugin identity and the project configuration path.
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

	// Build a plugin repository only at its reviewed, valid commit.
	if strings.HasPrefix(req.GetSourceKey(), pluginRepositoryKeyPrefix) {
		if err := r.checkPluginRepositoryBuild(ctx, tx, req); err != nil {
			return nil, err
		}
	}

	// Place the build on the selected Device's Worker.
	placement, err := r.selectDevicePlacement(ctx, tx, req.GetDeviceKey())
	if err != nil {
		return nil, err
	}

	// Pin the source directory's current root.
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
	return &pluginBuildSelection{devicePlacement: *placement, source: source, configPath: configPath}, nil
}

// lookupDevice reads the registered Device at key.
func lookupDevice(ctx context.Context, tx world.WorldState, key string) (*s4wave_device.Device, error) {
	// Require the object to be a Device.
	deviceType, err := world_types.GetObjectType(ctx, tx, key)
	if err != nil {
		return nil, err
	}
	if deviceType != s4wave_device.DeviceTypeID {
		return nil, errors.New("select a registered device")
	}

	// Read the Device block.
	device, object, err := world.LookupObject[*s4wave_device.Device](ctx, tx, key, s4wave_device.NewDeviceBlock)
	world.ReleaseObjectState(object)
	return device, err
}

// buildPlatform validates the requested platform against the Device.
// An empty request selects the Device's native platform; js is always allowed.
func (s *pluginBuildSelection) buildPlatform(requested string) (string, error) {
	// An empty request selects the Device's native platform.
	native := s.device.NativePlatformID()
	if requested == "" {
		requested = native
	}
	if requested == "" {
		return "", errors.New("device reports no native platform")
	}

	// Only js or the Device's platform can be built for this Device.
	if requested == bldr_platform.PlatformID_JS || requested == native {
		return requested, nil
	}
	return "", errors.Errorf("platform %q is neither js nor the device platform %q", requested, native)
}

// selectWorkerCluster finds the Cluster that will schedule a Job on the Worker.
// The Cluster must contain the Worker. If none is requested, the Worker must
// belong to exactly one.
func selectWorkerCluster(ctx context.Context, tx world.WorldState, workerKey, requested string) (string, error) {
	// List the Clusters that contain the Worker.
	clusters, err := forge_cluster.ListWorkerClusters(ctx, tx, workerKey)
	if err != nil {
		return "", err
	}

	// Use the requested Cluster or the Worker's only one.
	if requested != "" {
		if !slices.Contains(clusters, requested) {
			return "", errors.Errorf("worker %s is not linked to cluster %s", workerKey, requested)
		}
		return requested, nil
	}
	if len(clusters) != 1 {
		return "", errors.Errorf("worker %s belongs to %d clusters; select one", workerKey, len(clusters))
	}
	return clusters[0], nil
}

// checkPluginBuildCapacity requires an explicit capacity request that the
// Worker's observed totals can hold. A Worker that has not reported totals yet
// admits the request; its own admission decides when it starts.
func checkPluginBuildCapacity(ctx context.Context, tx world.WorldState, workerKey string, milliCPU, memoryBytes uint64) error {
	// Require an explicit request.
	if milliCPU == 0 || memoryBytes == 0 {
		return errors.New("milli_cpu and memory_bytes must be set")
	}

	// Compare it with the totals the Worker last reported.
	capacity, err := forge_runtime.LookupWorkerCapacity(ctx, tx, workerKey)
	if errors.Is(err, forge_runtime.ErrWorkerNotObserved) {
		return nil
	}
	if err != nil {
		return err
	}
	if milliCPU > capacity.MilliCPUTotal || memoryBytes > capacity.MemoryBytesTotal {
		return errors.Errorf("worker %s cannot hold %d milli-cores and %d bytes", workerKey, milliCPU, memoryBytes)
	}
	return nil
}

// queuedPluginFrontend identifies the accepted execution and assigned device Session.
type queuedPluginFrontend struct {
	// key is the durable Forge execution object key.
	key string
	// peer is the device's authenticated transport identity.
	peer peer.ID
}

// queuePluginFrontend places one live compiler attachment on the selected Device.
// The Execution is retained by the attaching Resource, not by a Job.
func (r *SpaceResource) queuePluginFrontend(ctx context.Context, req *s4wave_space.BuildSpacePluginRequest, config *space_exec.PluginBuildConfig) (*queuedPluginFrontend, error) {
	// Open the transaction and resolve the Device and pinned source.
	tx, err := r.space.GetWorldEngine().NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	selection, err := r.selectPluginBuild(ctx, tx, req)
	if err != nil {
		return nil, err
	}

	// The Execution owns the input DAG and survives the submitting client's lifetime.
	key := "plugin-builds/" + uuid.NewV4().String()
	config.ManifestId = req.GetManifestId()
	config.ConfigPath = selection.configPath
	config.FrontendPeerId = selection.sender.String()
	configData, err := config.MarshalJSON()
	if err != nil {
		return nil, err
	}

	// Create the Execution on the Device's Worker and commit it.
	_, err = forge_execution.CreateExecutionWithTarget(ctx, tx, selection.sender, key, selection.devicePeer,
		&forge_target.ValueSet{
			Inputs: forge_value.ValueSlice{forge_value.NewValueWithWorldObjectSnapshot("source", selection.source)},
		}, &forge_target.Target{
			Exec: &forge_target.Exec{
				Controller: &configset_proto.ControllerConfig{Id: space_exec.BuildPluginConfigID, Rev: 1, Config: configData},
			},
		}, selection.placement, timestamppb.Now())
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &queuedPluginFrontend{key: key, peer: selection.devicePeer}, nil
}
