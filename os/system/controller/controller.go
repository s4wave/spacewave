// Package os_system_controller runs a placed os/system node. It serves the
// release the node binds, and the Space's blocks, on its Device's bus, where
// the daemon socket routes the os.system services to it. The Device's console
// is the one client: it stages each new release into the image store it
// alone writes, and boots the staged revision on trial.
package os_system_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
	block_rpc_server "github.com/s4wave/spacewave/db/block/rpc/server"
	"github.com/s4wave/spacewave/db/world"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	os_release "github.com/s4wave/spacewave/os/release"
	os_system "github.com/s4wave/spacewave/os/system"
	os_system_stager "github.com/s4wave/spacewave/os/system/stager"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "os/system"

// Controller runs a placed os/system node. While its World engine is up it
// serves the Stager and the Space's blocks, read only, on the bus, and shows
// the release the node binds as its status. A release it cannot read shows
// as rejected. The console judges the signature, since only the Device's
// pins can.
type Controller struct {
	// le is the logger.
	le *logrus.Entry
	// b is the bus the services are published on.
	b bus.Bus
	// conf is the config.
	conf *os_system.Config
	// engine is the World engine holding the release.
	engine *world.BusEngine

	// status guards state and detail and wakes their watchers.
	status broadcast.Broadcast
	// state is the node's state.
	state s4wave_device.DeviceCapabilityState
	// detail describes state.
	detail string

	// reports guards revision and wakes its watchers.
	reports broadcast.Broadcast
	// revision is the console's last report of the Device's revisions, or
	// nil before its first. It outlives each engine, since the image store
	// does not change with the World.
	revision *os_system_stager.Revision
}

// NewController constructs a system controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *os_system.Config) *Controller {
	return &Controller{
		le:     le,
		b:      b,
		conf:   conf,
		engine: world.NewBusEngine(nil, b, conf.GetEngineId()),
		state:  s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE,
		detail: "starting",
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "OS system controller")
}

// Execute serves the release and the Space's blocks for each engine serving
// the World, until the context ends.
func (c *Controller) Execute(ctx context.Context) error {
	c.engine.SetContext(ctx)
	return world.AccessEngine(ctx, c.engine, c.serve)
}

// serve publishes the services over eng until ctx ends, and shows the
// release as the node's status meanwhile. The services leave the bus with
// eng, and each open stream ends with them.
func (c *Controller) serve(ctx context.Context, eng world.Engine) error {
	// Read the Space's blocks still encoded, as the image store keeps them.
	cs, err := eng.BuildStorageCursor(ctx)
	if err != nil {
		return errors.Wrap(err, "build storage cursor")
	}
	defer cs.Release()
	store := block.NewReadOnlyStore(cs.GetBlockStore())

	// Register the services. Staging copies a whole image, so its reads are
	// background work.
	mux := srpc.NewMux()
	if err := os_system_stager.SRPCRegisterStager(mux, &stager{ctx: ctx, eng: eng, key: c.conf.GetObjectKey(), c: c}); err != nil {
		return err
	}
	blocks := block_rpc_server.NewBlockStoreWithPolicy(store, block_rpc_server.ReadPolicy{Background: true})
	if err := mux.Register(block_rpc.NewSRPCBlockStoreHandler(blocks, os_system_stager.BlocksServiceID)); err != nil {
		return err
	}

	// Publish them on the bus for as long as eng serves.
	invoker := bifrost_rpc.NewInvokerController(c.le, c.b, c.GetControllerInfo(), mux, nil)
	release, err := c.b.AddController(ctx, invoker, nil)
	if err != nil {
		return err
	}
	defer release()

	// Show each release the node binds.
	return watchRelease(ctx, eng, c.conf.GetObjectKey(), func(r *os_release.Release, err error) error {
		c.showRelease(r, err)
		return nil
	})
}

// showRelease shows the release r, or the error reading it.
func (c *Controller) showRelease(r *os_release.Release, err error) {
	// Show the image root of a readable release.
	if err == nil {
		image, ierr := r.Image()
		if ierr == nil {
			c.setStatus(s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE, "release "+image.GetRootRef().MarshalString())
			return
		}
		err = ierr
	}

	// Wait for a missing release, and reject any other release error.
	if errors.Is(err, world.ErrObjectNotFound) {
		c.setStatus(s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE, "waiting for a release")
		return
	}
	c.le.WithError(err).Warn("cannot read the release")
	c.setStatus(s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_REJECTED, err.Error())
}

// GetNodeStatus returns the node's state and detail, and a channel closed at
// their next change.
func (c *Controller) GetNodeStatus() (s4wave_device.DeviceCapabilityState, string, <-chan struct{}) {
	l := c.status.Lock()
	defer l.Unlock()
	return c.state, c.detail, l.WaitCh()
}

// setStatus shows state and detail, waking the watchers when they change.
func (c *Controller) setStatus(state s4wave_device.DeviceCapabilityState, detail string) {
	// Store a changed status and wake its watchers. An unchanged status wakes
	// none.
	l := c.status.Lock()
	defer l.Unlock()
	if c.state == state && c.detail == detail {
		return
	}
	c.state, c.detail = state, detail
	l.Broadcast()
}

// setRevision stores the console's report, waking the watchers when it
// changes the revisions.
func (c *Controller) setRevision(r *os_system_stager.Revision) {
	// Store a changed report and wake the watchers.
	l := c.reports.Lock()
	defer l.Unlock()
	if c.revision.EqualVT(r) {
		return
	}
	c.revision = r
	l.Broadcast()
}

// getRevision returns the last report, or nil before the first report, and a
// channel closed at the next change.
func (c *Controller) getRevision() (*os_system_stager.Revision, <-chan struct{}) {
	l := c.reports.Lock()
	defer l.Unlock()
	return c.revision, l.WaitCh()
}

// HandleDirective resolves no directives. The services resolve through the
// invoker controller serve adds.
func (c *Controller) HandleDirective(context.Context, directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ controller.Controller                = (*Controller)(nil)
	_ s4wave_flowgraph.FlowgraphNodeStatus = (*Controller)(nil)
)
