package resource_worldop_registry

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_plugin "github.com/s4wave/spacewave/sdk/plugin"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
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
	// Match a named World operation lookup.
	dir, ok := di.GetDirective().(world.LookupWorldOp)
	if !ok {
		return nil, nil
	}
	opTypeID := dir.LookupWorldOpOperationTypeID()
	if opTypeID == "" {
		return nil, nil
	}

	// Proxy the operation to the pinned executable it names.
	engineID := dir.LookupWorldOpEngineID()
	pluginID, manifestRoot, handlerID, pinned := s4wave_worldop_registry.ParsePinnedOperationID(opTypeID)
	if pinned {
		reg := &s4wave_worldop_registry.WorldOpRegistration{PluginId: pluginID, OperationTypeId: handlerID}
		lookupOp := func(ctx context.Context, operationTypeID string) (world.Operation, error) {
			op := newBridgeOperation(c.le, c.b, reg, operationTypeID, engineID)
			op.manifestRoot = manifestRoot
			op.handlerID = handlerID
			return op, nil
		}
		return directive.R(world.NewLookupWorldOpResolver(lookupOp), nil)
	}

	// Follow the registered handler, so a replay waiting for a plugin resolves
	// once the plugin registers.
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		return c.watchRegistration(ctx, handler, opTypeID, engineID)
	}), nil)
}

// watchRegistration keeps handler's value a proxy to the plugin instance
// registered for opTypeID, and no value while none is registered.
func (c *WorldOpRegistryBridgeController) watchRegistration(
	ctx context.Context,
	handler directive.ResolverHandler,
	opTypeID, engineID string,
) error {
	// Follow the registration, holding the value of the one last seen.
	var valID, regID uint32
	var instanceKey string
	for {
		// Read the registration and the channel that signals its change.
		var reg *s4wave_worldop_registry.WorldOpRegistration
		var nextInstanceKey string
		var waitCh <-chan struct{}
		c.registry.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			reg, nextInstanceKey = c.registry.lookupPluginInstanceLocked(opTypeID, engineID)
			waitCh = getWaitCh()
		})

		// Replace the value when the registration or its instance changed.
		if reg.GetRegistrationId() != regID || nextInstanceKey != instanceKey {
			if valID != 0 {
				handler.RemoveValue(valID)
				valID = 0
			}
			regID, instanceKey = reg.GetRegistrationId(), nextInstanceKey
			if reg != nil {
				valID, _ = handler.AddValue(newRegisteredLookupOp(c.le, c.b, reg, instanceKey, engineID))
			}
		}
		handler.MarkIdle(true)

		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-waitCh:
		}
	}
}

// newRegisteredLookupOp returns a lookup that proxies each operation to the
// plugin instance instanceKey serving reg.
func newRegisteredLookupOp(
	le *logrus.Entry,
	b bus.Bus,
	reg *s4wave_worldop_registry.WorldOpRegistration,
	instanceKey, engineID string,
) world.LookupWorldOpValue {
	return func(ctx context.Context, operationTypeID string) (world.Operation, error) {
		op := newBridgeOperation(le, b, reg, operationTypeID, engineID)
		op.instanceKey = instanceKey
		return op, nil
	}
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
	// handlerID is the plugin-local operation name, separate from accepted code identity.
	handlerID string
	// instanceKey is the plugin instance serving reg, empty for a global one.
	instanceKey string
	// manifestRoot pins the executable; empty uses a registered native/legacy handler.
	manifestRoot string
	opData       []byte
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
		le:        le,
		b:         b,
		reg:       reg,
		engineID:  engineID,
		opTypeID:  opTypeID,
		handlerID: opTypeID,
	}
}

// TypeID returns the operation type identifier.
func (o *bridgeOperation) TypeID() string {
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

// MarshalVT marshals the block to binary. The bridge holds the encoded op
// bytes opaquely; the owning plugin defines the payload format.
func (o *bridgeOperation) MarshalVT() ([]byte, error) {
	return o.opData, nil
}

// UnmarshalVT unmarshals the block from binary. The engine decodes ops
// from block storage through this path; the bytes stay opaque until the
// plugin handler parses them.
func (o *bridgeOperation) UnmarshalVT(data []byte) error {
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
	// Connect to the plugin resources for the World operation call.
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

	// Acquire the plugin handler service for the duration of the call.
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

	// Validate the encoded World operation before exposing mutable state.
	if sysErr, err := o.validateWithService(ctx, svc); err != nil {
		return sysErr, err
	}

	// Attach a WorldStateResource so the TS handler can mutate world state.
	// Include built-in Space ops before the bus-backed dynamic registry so TS
	// handlers can call applyWorldOp recursively (e.g. to init UnixFS objects).
	lookupOp := space_world_optypes.BuildSpaceLookupOp(o.b, o.le, o.engineID)
	wsResource := resource_world.NewWorldStateResource(o.le, o.b, ws, lookupOp, resource_world.WithSessionPeerID(sender))
	typedResource := resource_world.NewTypedObjectResourceWithContext(
		objecttype.WithSessionPeerID(ctx, sender), o.le, o.b, ws, nil,
	)
	defer typedResource.Close()
	stateMux := srpc.NewMux(wsResource.GetMux())
	if err := s4wave_world.SRPCRegisterTypedObjectResourceService(stateMux, typedResource); err != nil {
		return true, err
	}

	// Attach the operation-scoped World services and detach them after the call.
	worldStateResourceID, err := resources.Client.AttachResource(ctx, "world-state", stateMux)
	if err != nil {
		return true, errors.Wrap(err, "attach world state resource")
	}
	defer func() {
		_ = resources.Client.DetachResource(ctx, worldStateResourceID)
	}()

	// Apply the encoded World operation through the attached state services.
	resp, err := svc.ApplyWorldOp(ctx, &s4wave_worldop_registry.ApplyWorldOpRequest{
		OperationTypeId:              o.handlerID,
		OpData:                       o.opData,
		AttachedWorldStateResourceId: worldStateResourceID,
		Sender:                       sender.String(),
	})
	if err != nil {
		return true, err
	}

	// Translate the plugin rejection into a World operation rejection.
	if resp.GetRejectionCode() != "" {
		return false, &world.OperationRejection{Code: resp.GetRejectionCode(), Message: resp.GetRejectionMessage()}
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
	// Connect to the plugin resources for the object operation call.
	resources, err := o.connectPlugin(ctx)
	if err != nil {
		return true, err
	}
	defer resources.Release()

	// Acquire the plugin handler service for the duration of the call.
	svc, cleanup, err := o.getHandlerService(resources)
	if err != nil {
		return true, err
	}
	defer cleanup()

	// Validate the encoded object operation before exposing mutable state.
	if sysErr, err := o.validateWithService(ctx, svc); err != nil {
		return sysErr, err
	}

	// Attach an ObjectStateResource so the TS handler can mutate the object and
	// apply nested object operations through the same lookup chain as world ops.
	lookupOp := space_world_optypes.BuildSpaceLookupOp(o.b, o.le, o.engineID)
	objResource := resource_world.NewObjectStateResource(o.le, o.b, os, lookupOp, resource_world.WithSessionPeerID(sender))
	objectStateResourceID, err := resources.Client.AttachResource(ctx, "object-state", objResource.GetMux())
	if err != nil {
		return true, errors.Wrap(err, "attach object state resource")
	}
	defer func() {
		_ = resources.Client.DetachResource(ctx, objectStateResourceID)
	}()

	// Apply the encoded object operation through the attached object service.
	resp, err := svc.ApplyWorldObjectOp(ctx, &s4wave_worldop_registry.ApplyWorldObjectOpRequest{
		OperationTypeId:               o.handlerID,
		OpData:                        o.opData,
		ObjectKey:                     os.GetKey(),
		AttachedObjectStateResourceId: objectStateResourceID,
		Sender:                        sender.String(),
	})
	if err != nil {
		return true, err
	}

	// Translate the plugin rejection into a World operation rejection.
	if resp.GetRejectionCode() != "" {
		// Translate the plugin rejection into a World operation rejection.
		return false, &world.OperationRejection{Code: resp.GetRejectionCode(), Message: resp.GetRejectionMessage()}
	}
	return resp.GetSystemError(), nil
}

func (o *bridgeOperation) validateWithService(
	ctx context.Context,
	svc s4wave_worldop_registry.SRPCWorldOpHandlerServiceClient,
) (bool, error) {
	resp, err := svc.ValidateOp(ctx, &s4wave_worldop_registry.ValidateOpRequest{
		OperationTypeId: o.handlerID,
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
	// Connect to the registering instance when no executable is pinned.
	if o.manifestRoot == "" {
		resources, err := s4wave_plugin.ConnectPluginInstanceResources(ctx, o.b, o.reg.GetPluginId(), o.instanceKey)
		if err != nil {
			return nil, errors.Wrap(err, "connect to plugin")
		}
		return resources, nil
	}

	// Reject the operation when its pinned executable is unavailable.
	resources, err := s4wave_plugin.ConnectPluginResourcesAtManifest(ctx, o.b, o.reg.GetPluginId(), o.instanceKey, o.manifestRoot)
	if err != nil {
		return nil, &world.OperationRejection{
			Code:    "UNAVAILABLE",
			Message: "The exact application executable is unavailable: " + o.manifestRoot,
		}
	}
	return resources, nil
}

// getHandlerService returns the WorldOpHandlerService client from the plugin root resource.
func (o *bridgeOperation) getHandlerService(resources *s4wave_plugin.PluginResources) (s4wave_worldop_registry.SRPCWorldOpHandlerServiceClient, func(), error) {
	// Access the plugin root service while retaining its resource reference.
	rootRef := resources.Client.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		return nil, nil, errors.Wrap(err, "get plugin root client")
	}

	// Return the handler client with cleanup for its retained root reference.
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
