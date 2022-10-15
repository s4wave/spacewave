package cluster_controller

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/controllerbus/util/keyed"
	forge_cluster "github.com/aperturerobotics/forge/cluster"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/cluster/1"

// Controller implements the Cluster controller.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the execution controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// objKey is the object key (from the config)
	objKey string
	// peerID is the parsed peer id
	peerID peer.ID
	// peerIDStr is the peer id string
	peerIDStr string

	// objLoop is the object tracking loop
	objLoop *world_control.ObjectLoop
	// jobTrackers manages the list of job tracker routines.
	jobTrackers *keyed.Keyed[*jobTracker]
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	peerID, _ := conf.ParsePeerID()
	c := &Controller{
		le:        le,
		bus:       bus,
		conf:      conf,
		objKey:    conf.GetObjectKey(),
		peerID:    peerID,
		peerIDStr: peerID.Pretty(),
	}
	c.objLoop = world_control.NewObjectLoop(
		le.WithField("object-loop", "cluster-controller"),
		c.objKey,
		c.ProcessState,
	)
	c.jobTrackers = keyed.NewKeyedWithLogger(c.newJobTracker, le)
	return c
}

// StartControllerWithConfig starts a controller with a config.
// Waits for the controller to start.
// Returns a Release function to close the controller when done.
func StartControllerWithConfig(
	ctx context.Context,
	b bus.Bus,
	conf *Config,
) (*Controller, directive.Reference, error) {
	ctrli, _, ctrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(conf),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	cl, ok := ctrli.(*Controller)
	if !ok {
		return nil, nil, block.ErrUnexpectedType
	}
	return cl, ctrlRef, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"cluster controller",
	)
}

// Execute executes the controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(rctx context.Context) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	c.jobTrackers.SetContext(ctx, true)
	return world_control.ExecuteBusObjectLoop(ctx, c.bus, c.conf.GetEngineId(), true, c.objLoop)
}

// ProcessState implements the state reconciliation loop.
func (c *Controller) ProcessState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	objKey := c.objKey
	if obj == nil {
		le.Debug("object does not exist, waiting")
		return true, nil
	}

	// check the <type> of the cluster objects
	typesState := world_types.NewTypesState(ctx, ws)
	err = forge_cluster.CheckClusterType(typesState, objKey)
	if err != nil {
		return false, err
	}

	// unmarshal Cluster state
	var clusterState *forge_cluster.Cluster
	_, err = world.AccessObject(ctx, ws.AccessWorldState, rootRef, func(bcs *block.Cursor) error {
		var berr error
		clusterState, berr = forge_cluster.UnmarshalCluster(bcs)
		if berr == nil {
			berr = clusterState.Validate()
		}
		return berr
	})
	if err != nil {
		return true, err
	}

	// determine the next operations to apply to the Cluster
	// scan list of active Job linked to the Cluster
	jobKeys, err := forge_cluster.ListClusterJobs(ctx, ws)
	if err != nil {
		return true, err
	}

	// assign all job to job trackers
	c.jobTrackers.SyncKeys(jobKeys, true)
	return true, nil
}

// _ is a type assertion
var _ world_control.ObjectLoopHandler = ((*Controller)(nil)).ProcessState

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
// The context clustered is canceled when the directive instance expires.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
