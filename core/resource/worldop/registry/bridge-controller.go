package resource_worldop_registry

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_plugin "github.com/s4wave/spacewave/sdk/plugin"
	s4wave_worldop_registry "github.com/s4wave/spacewave/sdk/worldop/registry"
	"github.com/sirupsen/logrus"
)

// bridgeControllerID is the controller ID.
const bridgeControllerID = "resource/worldop-registry-bridge"

// bridgeControllerVersion is the controller version.
var bridgeControllerVersion = controller.MustParseVersion("0.0.1")

// WorldOpRegistryBridgeController resolves LookupWorldOp directives for
// registry-registered operation types by proxying to the owning plugin.
type WorldOpRegistryBridgeController struct {
	le       *logrus.Entry
	b        bus.Bus
	registry *WorldOpRegistryResource
}

// NewWorldOpRegistryBridgeController creates a new WorldOpRegistryBridgeController.
func NewWorldOpRegistryBridgeController(
	le *logrus.Entry,
	b bus.Bus,
	registry *WorldOpRegistryResource,
) *WorldOpRegistryBridgeController {
	return &WorldOpRegistryBridgeController{
		le:       le,
		b:        b,
		registry: registry,
	}
}

// GetControllerInfo returns controller info.
func (c *WorldOpRegistryBridgeController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(bridgeControllerID, bridgeControllerVersion, "worldop registry bridge controller")
}

// Execute executes the controller.
func (c *WorldOpRegistryBridgeController) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *WorldOpRegistryBridgeController) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(world.LookupWorldOp)
	if !ok {
		return nil, nil
	}
	opTypeID := dir.LookupWorldOpOperationTypeID()
	if opTypeID == "" {
		return nil, nil
	}
	reg := c.registry.LookupRegistrationByOpType(opTypeID)
	if reg == nil {
		return nil, nil
	}
	engineID := dir.LookupWorldOpEngineID()
	lookupOp := func(ctx context.Context, operationTypeID string) (world.Operation, error) {
		return newBridgeOperation(c.le, c.b, reg, operationTypeID, engineID), nil
	}
	return directive.R(world.NewLookupWorldOpResolver(lookupOp), nil)
}

// Close releases any resources held by the controller.
func (c *WorldOpRegistryBridgeController) Close() error {
	return nil
}

// bridgeOperation implements world.Operation by proxying to the owning plugin.
type bridgeOperation struct {
	le       *logrus.Entry
	b        bus.Bus
	reg      *s4wave_worldop_registry.WorldOpRegistration
	engineID string
	opTypeID string
	opData   []byte
}

// newBridgeOperation creates a new bridgeOperation.
func newBridgeOperation(
	le *logrus.Entry,
	b bus.Bus,
	reg *s4wave_worldop_registry.WorldOpRegistration,
	opTypeID string,
	engineID string,
) *bridgeOperation {
	return &bridgeOperation{
		le:       le,
		b:        b,
		reg:      reg,
		engineID: engineID,
		opTypeID: opTypeID,
	}
}

// GetOperationTypeId returns the operation type identifier.
func (o *bridgeOperation) GetOperationTypeId() string {
	return o.opTypeID
}

func bridgeOperationEngineID(op world.Operation) (string, bool) {
	bridgeOp, ok := op.(*bridgeOperation)
	if !ok {
		return "", false
	}
	return bridgeOp.engineID, true
}

// Validate accepts bridge operations after local block decoding. Plugin
// validation needs the request context used by ApplyWorldOp or ApplyWorldObjectOp.
func (o *bridgeOperation) Validate() error {
	return nil
}

// MarshalBlock marshals the block to binary. The bridge holds the encoded op
// bytes opaquely; the owning plugin defines the payload format.
func (o *bridgeOperation) MarshalBlock() ([]byte, error) {
	return o.opData, nil
}

// UnmarshalBlock unmarshals the block from binary. The engine decodes ops
// from block storage through this path; the bytes stay opaque until the
// plugin handler parses them.
func (o *bridgeOperation) UnmarshalBlock(data []byte) error {
	o.opData = data
	return nil
}

// ApplyWorldOp applies the operation as a world operation by proxying to the TS plugin.
func (o *bridgeOperation) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (bool, error) {
	resources, err := o.connectPlugin(ctx)
	if err != nil {
		return true, errors.Wrapf(
			err,
			"apply world op plugin_id=%s capability=world_state path=ApplyWorldOp operation_type_id=%s",
			o.reg.GetPluginId(),
			o.opTypeID,
		)
	}
	defer resources.Release()

	svc, cleanup, err := o.getHandlerService(resources)
	if err != nil {
		return true, errors.Wrapf(
			err,
			"apply world op plugin_id=%s capability=world_state path=ApplyWorldOp operation_type_id=%s",
			o.reg.GetPluginId(),
			o.opTypeID,
		)
	}
	defer cleanup()

	if sysErr, err := o.validateWithService(ctx, svc); err != nil {
		return sysErr, err
	}

	// Attach a WorldStateResource so the TS handler can mutate world state.
	// Include built-in Space ops before the bus-backed dynamic registry so TS
	// handlers can call applyWorldOp recursively (e.g. to init UnixFS objects).
	lookupOp := space_world_optypes.BuildSpaceLookupOp(o.b, o.le, o.engineID)
	wsResource := resource_world.NewWorldStateResource(o.le, o.b, ws, lookupOp)
	worldStateResourceID, err := resources.Client.AttachResource(ctx, "world-state", wsResource.GetMux())
	if err != nil {
		return true, errors.Wrap(err, "attach world state resource")
	}
	defer func() {
		_ = resources.Client.DetachResource(ctx, worldStateResourceID)
	}()

	resp, err := svc.ApplyWorldOp(ctx, &s4wave_worldop_registry.ApplyWorldOpRequest{
		OperationTypeId:              o.opTypeID,
		OpData:                       o.opData,
		AttachedWorldStateResourceId: worldStateResourceID,
	})
	if err != nil {
		return true, err
	}
	return resp.GetSystemError(), nil
}

// ApplyWorldObjectOp applies the operation to a world object by proxying to the TS plugin.
func (o *bridgeOperation) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (bool, error) {
	resources, err := o.connectPlugin(ctx)
	if err != nil {
		return true, err
	}
	defer resources.Release()

	svc, cleanup, err := o.getHandlerService(resources)
	if err != nil {
		return true, err
	}
	defer cleanup()

	if sysErr, err := o.validateWithService(ctx, svc); err != nil {
		return sysErr, err
	}

	// Attach an ObjectStateResource so the TS handler can mutate the object and
	// apply nested object operations through the same lookup chain as world ops.
	lookupOp := space_world_optypes.BuildSpaceLookupOp(o.b, o.le, o.engineID)
	objResource := resource_world.NewObjectStateResource(o.le, o.b, os, lookupOp)
	objectStateResourceID, err := resources.Client.AttachResource(ctx, "object-state", objResource.GetMux())
	if err != nil {
		return true, errors.Wrap(err, "attach object state resource")
	}
	defer func() {
		_ = resources.Client.DetachResource(ctx, objectStateResourceID)
	}()

	resp, err := svc.ApplyWorldObjectOp(ctx, &s4wave_worldop_registry.ApplyWorldObjectOpRequest{
		OperationTypeId:               o.opTypeID,
		OpData:                        o.opData,
		ObjectKey:                     os.GetKey(),
		AttachedObjectStateResourceId: objectStateResourceID,
	})
	if err != nil {
		return true, err
	}
	return resp.GetSystemError(), nil
}

func (o *bridgeOperation) validateWithService(
	ctx context.Context,
	svc s4wave_worldop_registry.SRPCWorldOpHandlerServiceClient,
) (bool, error) {
	resp, err := svc.ValidateOp(ctx, &s4wave_worldop_registry.ValidateOpRequest{
		OperationTypeId: o.opTypeID,
		OpData:          o.opData,
	})
	if err != nil {
		return true, err
	}
	if msg := resp.GetError(); msg != "" {
		return false, errors.New(msg)
	}
	return false, nil
}

// connectPlugin connects to the TS plugin's resource service.
func (o *bridgeOperation) connectPlugin(ctx context.Context) (*s4wave_plugin.PluginResources, error) {
	resources, err := s4wave_plugin.ConnectPluginResources(ctx, o.b, o.reg.GetPluginId())
	if err != nil {
		return nil, errors.Wrap(err, "connect to plugin")
	}
	return resources, nil
}

// getHandlerService returns the WorldOpHandlerService client from the plugin root resource.
func (o *bridgeOperation) getHandlerService(resources *s4wave_plugin.PluginResources) (s4wave_worldop_registry.SRPCWorldOpHandlerServiceClient, func(), error) {
	rootRef := resources.Client.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		return nil, nil, errors.Wrap(err, "get plugin root client")
	}

	svc := s4wave_worldop_registry.NewSRPCWorldOpHandlerServiceClient(rootClient)
	cleanup := func() {
		rootRef.Release()
	}
	return svc, cleanup, nil
}

// _ is a type assertion
var _ world.Operation = (*bridgeOperation)(nil)

// _ is a type assertion
var _ controller.Controller = (*WorldOpRegistryBridgeController)(nil)
