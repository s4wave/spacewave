package forge_lib_podman_pod

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/aperturerobotics/containers/podman"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	forge_target "github.com/aperturerobotics/forge/target"
	pdefine "github.com/containers/podman/v4/libpod/define"
	pentities "github.com/containers/podman/v4/pkg/domain/entities"
	"sigs.k8s.io/yaml"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	// apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"

	k8s_v1 "k8s.io/api/core/v1"
	k8s_metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/lib/podman/pod/1"

const (
	// inputNameWorld is the name of the Input for the target World.
	inputNameWorld = "world"
)

// Controller implements the podman pod controller.
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

// NewController constructs a new podman pod controller.
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
func (c *Controller) GetControllerInfo() controller.Info {
	return controller.Info{
		Id:      ControllerID,
		Version: Version.String(),
	}
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
	inWorld := c.inputVals[inputNameWorld]

	// ws will be nil if the world state was not set
	ws, err := forge_target.InputValueToWorldState(inWorld)
	if err != nil {
		return errors.Wrap(err, "world")
	}
	_ = ws

	// build the pod metadata
	uniqueID := c.handle.GetExecutionUniqueId()
	podName := "forge-exec-" + strings.ToLower(uniqueID[:8])
	podObj := &k8s_v1.Pod{
		TypeMeta: k8s_metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: k8s_metav1.ObjectMeta{
			Name: podName,
			Labels: map[string]string{
				"forge/exec": uniqueID,
				"forge/peer": c.handle.GetPeerId().Pretty(),
			},
		},
	}

	// decode and check the spec
	c.le.Debug("podman: validating spec")
	err = c.conf.ParseSpec(&podObj.Spec)
	if err != nil {
		return errors.Wrap(err, "spec")
	}
	volsMap, err := c.conf.BuildVolumeMap(&podObj.Spec)
	if err != nil {
		return err
	}

	// process the world volumes
	// only mount the volumes that are actually used
	// replace the volumes in the spec with hostPath
	worldVolumes := c.conf.GetWorldVolumes()
	for volID, k8sVols := range volsMap {
		worldVolume := worldVolumes[volID]
		unixfsRef := worldVolume.ToUnixfsRef()
		c.le.Debugf("TODO: mount world volume %s: object %s", volID, unixfsRef.GetObjectKey())

		// TODO update the volumes to use
		for _, vol := range k8sVols {
			vol.HostPath = &k8s_v1.HostPathVolumeSource{
				// TODO: does not exist
				Path: "/tmp/path-to-the-host-volume/" + volID,
			}
			vol.PersistentVolumeClaim = nil
		}
	}

	// encode the pod spec back to yaml
	podYAML, err := yaml.Marshal(podObj)
	if err != nil {
		return err
	}

	c.le.Debug("podman: waiting for engine")
	engine, engineRef, err := podman.ExLookupPodman(ctx, c.bus, c.conf.GetPodmanId())
	if err != nil {
		return err
	}
	defer engineRef.Release()

	runKubeDown := func(ctx context.Context) error {
		report, err := engine.PlayKubeDown(ctx, bytes.NewReader(podYAML), pentities.PlayKubeDownOptions{})
		if err != nil {
			// https://github.com/containers/podman/issues/13730
			if !strings.HasPrefix(err.Error(), "json: cannot unmarshal string") {
				c.le.WithError(err).Warn("unable to bring down pod")
			}
		}
		_ = report
		return err
	}

	// Check if the pod already exists.
	c.le.Debug("podman: checking if pod exists")
	podExists, err := engine.PodExists(ctx, podObj.Name)
	if err != nil {
		return err
	}
	if podExists.Value {
		c.le.Debug("podman: bringing down old pod version")
		if err := runKubeDown(ctx); err != nil {
			return err
		}
	}

	c.le.Debug("podman: creating pod")
	report, err := engine.PlayKube(ctx, bytes.NewReader(podYAML), pentities.PlayKubeOptions{
		// Note: this option is ignored by the podman code.
		// https://github.com/containers/podman/issues/13663
		Replace: true,
	})
	if err != nil {
		return errors.Wrap(err, "podman: play kube pod")
	}

	defer func() {
		_ = runKubeDown(context.Background())
	}()

	if len(report.Pods) != 1 {
		c.le.Errorf("expected 1 pod but podman created %d", len(report.Pods))
		return errors.New("failed to create podman pod")
	}

	runningPod := &report.Pods[0]
	podID := runningPod.ID
	runningContainersIDs := runningPod.Containers

WaitContainer:
	le := c.le.
		WithField("pod-id", podID).
		WithField("pod-name", podName)
	le.Debugf(
		"podman: waiting for pod container(s) to exit: %v",
		// len(runningContainersIDs),
		runningContainersIDs,
	)
	waitReport, err := engine.ContainerWait(
		ctx,
		runningContainersIDs,
		pentities.WaitOptions{
			Interval: time.Millisecond * 250,
			Condition: []pdefine.ContainerStatus{
				pdefine.ContainerStateExited,
				pdefine.ContainerStateStopped,
			},
		},
	)
	if err != nil {
		return err
	}

	var podErr error
	checkErr := func(err error, exitCode int32) error {
		if podErr != nil {
			return podErr
		}
		if err != nil {
			podErr = err
		} else if exitCode != 0 {
			podErr = errors.Errorf("container exited with code: %d", exitCode)
		}
		return podErr
	}

	for _, res := range waitReport {
		if checkErr(res.Error, res.ExitCode) != nil {
			break
		}
	}

	var failedToStart []string
	if podErr == nil {
		inspectReport, inspectErrs, err := engine.ContainerInspect(ctx, runningContainersIDs, pentities.InspectOptions{})
		if err != nil {
			return err
		}
		for i, inspectErr := range inspectErrs {
			if inspectErr != nil {
				podErr = errors.Wrapf(inspectErr, "error inspecting container %s", runningContainersIDs[i])
			}
			if podErr != nil {
				break
			}
		}
		for _, rep := range inspectReport {
			if podErr != nil {
				break
			}
			if rep == nil || rep.State == nil {
				continue
			}
			exitCode := rep.State.ExitCode
			if checkErr(nil, exitCode) != nil {
				break
			}
			status := rep.State.Status
			if status == pdefine.ContainerStateCreated.String() ||
				status == pdefine.ContainerStateConfigured.String() {
				// The container most likely did not start properly.
				failedToStart = append(failedToStart, rep.ID)
			}
		}
	}

	if podErr == nil && len(failedToStart) != 0 {
		le.WithError(err).Warnf("container failed to start: %v", failedToStart)

		// We cannot determine the error from the inspect report:
		// See: https://github.com/containers/podman/issues/13729
		startRep, err := engine.ContainerStart(ctx, failedToStart, pentities.ContainerStartOptions{})
		if err == nil && startRep == nil {
			err = errors.New("container start returned empty response")
		}
		if err == nil {
			for _, rep := range startRep {
				err = checkErr(rep.Err, int32(rep.ExitCode))
			}
		}
		if err != nil {
			podErr = errors.Wrap(err, "container failed to start")
		} else {
			// the containers started successfully?
			le.Warnf("started stalled containers: %v", failedToStart)
			goto WaitContainer
		}
	}

	if err := podErr; err != nil {
		le.WithError(err).Warn("pod exited with error")
	} else {
		le.Debug("pod exited successfully")
	}

	// done
	return podErr
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, inst directive.Instance) (directive.Resolver, error) {
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
