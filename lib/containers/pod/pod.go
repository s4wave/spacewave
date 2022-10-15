package forge_lib_containers_pod

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/aperturerobotics/containers/pod"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	forge_target "github.com/aperturerobotics/forge/target"
	"github.com/aperturerobotics/hydra/world"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	k8s_metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/lib/containers/pod/1"

const (
	// inputNameWorld is the name of the Input for the default World.
	inputNameWorld = "world"
)

// Controller implements the containers pod controller.
type Controller struct {
	// le is the log entry
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the configuration
	conf *Config
	// inputVals is the input values map
	inputVals forge_target.InputMap
	// handle contains the controller handle
	handle forge_target.ExecControllerHandle
}

// NewController constructs a new containers pod controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	return &Controller{
		le:   le,
		bus:  bus,
		conf: conf,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"pod controller",
	)
}

// InitForgeExecController initializes the Forge execution controller.
// This is called before Execute().
// Any error returned cancels execution of the controller.
func (c *Controller) InitForgeExecController(
	ctx context.Context,
	inputVals forge_target.InputMap,
	handle forge_target.ExecControllerHandle,
) error {
	c.inputVals, c.handle = inputVals, handle
	return c.conf.Validate()
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// lookup the world engine
	// sender := c.handle.GetPeerId()
	// ts := c.handle.GetTimestamp()

	// copy the pod conf object
	podConf := c.conf.GetPod()
	if podConf == nil {
		return errors.New("pod config cannot be empty")
	}

	podConf = podConf.Clone()
	if podConf.WorldVolumes == nil {
		podConf.WorldVolumes = make(map[string]*pod.WorldVolume)
	}

	// volumeWorld maps engine_id to WorldState.
	volumeWorlds := make(map[string]world.WorldState)

	// re-map the pod world volume engine ids
	var err error
	for _, vol := range podConf.GetWorldVolumes() {
		volEngineID := vol.EngineId
		if volEngineID == "" {
			volEngineID = inputNameWorld
		}

		// resolve the corresponding input
		inWorld := c.inputVals[volEngineID]
		ws := volumeWorlds[volEngineID]
		if ws == nil {
			ws, err = forge_target.InputValueToWorldState(inWorld)
			if err != nil {
				return errors.Wrap(err, "world")
			}
			volumeWorlds[volEngineID] = ws
		}
		if ws == nil {
			return errors.Errorf("world input not found for volume: %s", volEngineID)
		}

		vol.EngineId = volEngineID
	}

	// process any volume_inputs
	for volumeInputID, worldVolumeID := range c.conf.GetVolumeInputs() {
		inVal, ok := c.inputVals[volumeInputID]
		if !ok {
			return errors.Errorf("world_volumes[%s]: input value not set", volumeInputID)
		}

		wo, err := forge_target.InputValueToWorldObject(inVal)
		if err != nil {
			return errors.Wrapf(err, "world_volumes[%s]", volumeInputID)
		}

		obj := wo.GetWorldObject()
		if obj == nil {
			return errors.Errorf("world_volumes[%s]: object must be set", volumeInputID)
		}

		volumeWorlds[worldVolumeID] = wo.GetWorldState()
		podConf.WorldVolumes[worldVolumeID] = &pod.WorldVolume{
			EngineId:  worldVolumeID,
			ObjectKey: obj.GetKey(),
		}
	}

	var objMeta k8s_metav1.ObjectMeta
	if err := c.conf.ParseMeta(&objMeta); err != nil {
		return errors.Wrap(err, "meta")
	}
	if objMeta.Labels == nil {
		objMeta.Labels = make(map[string]string)
	}

	uniqueID := c.handle.GetExecutionUniqueId()
	podUUID := strings.ToLower(uniqueID[:8])

	objMeta.Labels["forge/type"] = "exec"
	objMeta.Labels["forge/exec"] = uniqueID
	objMeta.Labels["forge/peer"] = c.handle.GetPeerId().Pretty()

	// If the user set a name or generateName, append to it.
	baseName := objMeta.GenerateName
	if objMeta.Name != "" {
		baseName = objMeta.Name
	}
	var podName string
	if baseName != "" {
		podName = strings.Join([]string{baseName, podUUID}, "-")
	} else {
		podName = podUUID
	}
	podName = strings.Join([]string{"forge", podName}, "-")
	objMeta.Name = podName
	objMeta.GenerateName = ""

	// TODO: logs -> forge world objects (streams via File)
	// io.MultiWriter(writers ...io.Writer)
	var stdout, stderr io.Writer
	if !c.conf.GetQuiet() {
		stdout, stderr = os.Stdout, os.Stderr
	}

	podPeerID, err := c.conf.ParsePeerID()
	if err != nil {
		return errors.Wrap(err, "peer_id")
	}
	if len(podPeerID) == 0 {
		podPeerID = c.handle.GetPeerId()
	}

	val, err := pod.ExExecutePod(
		ctx,
		c.bus,
		podPeerID,
		c.conf.GetEngineId(),
		objMeta,
		podConf,
		stdout, stderr,
		volumeWorlds,
	)
	if err == nil && val != nil {
		err = val.GetError()
	}

	return err
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

/*
	errs := k8s_validation.ValidatePodSpec(spec, nil, nil, k8s_validation.PodValidationOptions{})
	if len(errs) != 0 {
		return errors.Errorf("pod spec: validation failed: %v", errs)
	}
*/

// _ is a type assertion
var _ forge_target.ExecController = ((*Controller)(nil))
