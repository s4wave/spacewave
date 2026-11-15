package resource_worldop_registry

import (
	"context"
	"fmt"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_plugin "github.com/s4wave/spacewave/sdk/plugin"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	s4wave_worldop_registry "github.com/s4wave/spacewave/sdk/worldop/registry"
	"github.com/sirupsen/logrus"
)

// pluginOpDeadline bounds the calls into a plugin's worker for one World
// operation, validate, attach, apply and detach, when the operation runs with
// no started world.WriteBudget, such as on a rehearsal or a fork. An operation
// applied in an engine write that holds the World engine's writer instead ends
// its calls at the write's budget deadline, which every attempt and nested
// operation of the write shares, so a worker that stalls but stays alive does
// not block every write to the Space until the caller gives up.
//
// An operation applied through an engine World state connects to its plugin
// before the writer, in PrepareApply, under pluginLoadDeadline instead. One
// that was not prepared, in a replay or inside a caller's transaction,
// connects under the same deadline, so a cold load that hangs ends with it.
const pluginOpDeadline = world.WriteBudgetLength

// pluginLoadDeadline bounds the plugin load PrepareApply waits on before the
// writer. A cold load fetches and starts the plugin's executable, which can
// legitimately outlast pluginOpDeadline; it holds no writer, so only the
// caller waits. A load that has not finished by then reports the plugin
// unavailable rather than leaving the caller waiting on a load that hangs.
const pluginLoadDeadline = 30 * time.Second

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
	// loadDeadline bounds each operation's plugin load in PrepareApply.
	loadDeadline time.Duration
}

// BridgeOption configures a WorldOpRegistryBridgeController.
type BridgeOption func(*WorldOpRegistryBridgeController)

// WithPluginLoadDeadline sets how long PrepareApply waits for an operation's
// plugin to load before it reports the plugin unavailable. The default is
// pluginLoadDeadline.
func WithPluginLoadDeadline(d time.Duration) BridgeOption {
	return func(c *WorldOpRegistryBridgeController) {
		c.loadDeadline = d
	}
}

// NewWorldOpRegistryBridgeController creates a new WorldOpRegistryBridgeController.
func NewWorldOpRegistryBridgeController(
	le *logrus.Entry,
	b bus.Bus,
	registry *WorldOpRegistryResource,
	opts ...BridgeOption,
) *WorldOpRegistryBridgeController {
	c := &WorldOpRegistryBridgeController{
		le:           le,
		b:            b,
		registry:     registry,
		loadDeadline: pluginLoadDeadline,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
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
			op := newBridgeOperation(c.le, c.b, reg, operationTypeID, engineID, c.loadDeadline)
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
				valID, _ = handler.AddValue(newRegisteredLookupOp(c.le, c.b, reg, instanceKey, engineID, c.loadDeadline))
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
	loadDeadline time.Duration,
) world.LookupWorldOpValue {
	return func(ctx context.Context, operationTypeID string) (world.Operation, error) {
		op := newBridgeOperation(le, b, reg, operationTypeID, engineID, loadDeadline)
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
	// loadDeadline bounds the plugin load in PrepareApply.
	loadDeadline time.Duration
	// prepared is the plugin connection PrepareApply made, until its release.
	prepared *s4wave_plugin.PluginResources
}

// newBridgeOperation creates a new bridgeOperation.
func newBridgeOperation(
	le *logrus.Entry,
	b bus.Bus,
	reg *s4wave_worldop_registry.WorldOpRegistration,
	opTypeID string,
	engineID string,
	loadDeadline time.Duration,
) *bridgeOperation {
	return &bridgeOperation{
		le:           le,
		b:            b,
		reg:          reg,
		engineID:     engineID,
		opTypeID:     opTypeID,
		handlerID:    opTypeID,
		loadDeadline: loadDeadline,
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
//
// The handler's writes are guarded as its plugin's. An op the Space replays
// from its log records the writes the guard would refuse instead, since its
// author's run already admitted it.
func (o *bridgeOperation) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (bool, error) {
	guard := newHandlerWriteGuard(
		o.le,
		space_world_optypes.NewPluginWriteGuard(o.reg.GetPluginId()),
		o.opTypeID,
		space_world.IsReplay(ctx),
	)
	return o.applyWorldOp(ctx, ws, sender, guard)
}

// applyWorldOp applies the op through its plugin's handler, with every write
// the handler makes checked by guard.
func (o *bridgeOperation) applyWorldOp(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	guard *handlerWriteGuard,
) (bool, error) {
	// Bound the plugin's load, when not prepared, and every call into its
	// worker while the writer is held.
	ctx, cancel := o.withDeadline(ctx)
	defer cancel()

	// Use the prepared plugin connection, or connect.
	resources, release, err := o.acquirePlugin(ctx)
	if err != nil {
		return true, errors.Wrapf(
			o.callErr(ctx, err, false),
			"apply world op plugin_id=%s capability=world_state path=ApplyWorldOp operation_type_id=%s",
			o.reg.GetPluginId(),
			o.opTypeID,
		)
	}
	defer release()

	// Acquire the plugin handler service for the duration of the call.
	svc, cleanup, err := o.getHandlerService(resources)
	if err != nil {
		return true, errors.Wrapf(
			o.callErr(ctx, err, false),
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
	// The handler writes as its own plugin, whoever applied the op.
	lookupOp := space_world_optypes.BuildSpaceLookupOp(o.b, o.le, o.engineID)
	opts := []resource_world.WorldStateResourceOption{
		resource_world.WithSessionPeerID(sender),
		resource_world.WithWriteGuard(guard),
	}
	wsResource := resource_world.NewWorldStateResource(o.le, o.b, ws, lookupOp, opts...)
	typedResource := resource_world.NewTypedObjectResourceWithContext(
		objecttype.WithSessionPeerID(ctx, sender), o.le, o.b, ws, nil, opts...,
	)
	defer typedResource.Close()
	stateMux := srpc.NewMux(wsResource.GetMux())
	if err := s4wave_world.SRPCRegisterTypedObjectResourceService(stateMux, typedResource); err != nil {
		return true, err
	}

	// Attach the operation-scoped World services and detach them after the call.
	worldStateResourceID, err := resources.Client.AttachResource(ctx, "world-state", stateMux)
	if err != nil {
		return true, o.callErr(ctx, errors.Wrap(err, "attach world state resource"), false)
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

	// Fail the op on a refused or undecided write, even one the handler caught
	// or returned over RPC, which drops the error's type.
	if sysErr, gerr := guard.result(); gerr != nil {
		return sysErr, gerr
	}
	if err != nil {
		return true, o.callErr(ctx, err, true)
	}

	// Translate the plugin rejection into a World operation rejection.
	if resp.GetRejectionCode() != "" {
		return false, &world.OperationRejection{Code: resp.GetRejectionCode(), Message: resp.GetRejectionMessage()}
	}
	if resp.GetSystemError() {
		return true, world.NewOpSystemError(nil)
	}
	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object by proxying to the
// TS plugin. The handler's writes to the object are guarded as ApplyWorldOp
// guards a World op's: as its plugin's, recorded instead of refused on replay.
func (o *bridgeOperation) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (bool, error) {
	// The guard reads the World that holds the object.
	holder, ok := os.(world.WorldStateGetter)
	if !ok {
		return true, errors.Errorf("apply world object op operation_type_id=%s: object state %T does not report its World", o.opTypeID, os)
	}
	guard := newHandlerWriteGuard(
		o.le,
		space_world_optypes.NewPluginWriteGuard(o.reg.GetPluginId()),
		o.opTypeID,
		space_world.IsReplay(ctx),
	)
	return o.applyWorldObjectOp(ctx, world.GuardObjectState(os, holder.GetWorldState(), guard), sender, guard)
}

// applyWorldObjectOp applies the op to os through its plugin's handler, with
// every write the handler makes checked by guard, which os carries.
func (o *bridgeOperation) applyWorldObjectOp(
	ctx context.Context,
	os world.ObjectState,
	sender peer.ID,
	guard *handlerWriteGuard,
) (bool, error) {
	// Bound the plugin's load, when not prepared, and every call into its
	// worker while the writer is held.
	ctx, cancel := o.withDeadline(ctx)
	defer cancel()

	// Use the prepared plugin connection, or connect.
	resources, release, err := o.acquirePlugin(ctx)
	if err != nil {
		return true, o.callErr(ctx, err, false)
	}
	defer release()

	// Acquire the plugin handler service for the duration of the call.
	svc, cleanup, err := o.getHandlerService(resources)
	if err != nil {
		return true, o.callErr(ctx, err, false)
	}
	defer cleanup()

	// Validate the encoded object operation before exposing mutable state.
	if sysErr, err := o.validateWithService(ctx, svc); err != nil {
		return sysErr, err
	}

	// Attach an ObjectStateResource so the TS handler can mutate the object and
	// apply nested object operations through the same lookup chain as world ops.
	// The handler reaches only os, the object its caller targeted.
	lookupOp := space_world_optypes.BuildSpaceLookupOp(o.b, o.le, o.engineID)
	objResource := resource_world.NewObjectStateResource(o.le, o.b, os, lookupOp, resource_world.WithSessionPeerID(sender))
	objectStateResourceID, err := resources.Client.AttachResource(ctx, "object-state", objResource.GetMux())
	if err != nil {
		return true, o.callErr(ctx, errors.Wrap(err, "attach object state resource"), false)
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

	// Fail the op on a refused or undecided write, even one the handler caught
	// or returned over RPC, which drops the error's type.
	if sysErr, gerr := guard.result(); gerr != nil {
		return sysErr, gerr
	}
	if err != nil {
		return true, o.callErr(ctx, err, true)
	}

	// Translate the plugin rejection into a World operation rejection.
	if resp.GetRejectionCode() != "" {
		return false, &world.OperationRejection{Code: resp.GetRejectionCode(), Message: resp.GetRejectionMessage()}
	}
	if resp.GetSystemError() {
		return true, world.NewOpSystemError(nil)
	}
	return false, nil
}

// PrepareApply connects to the plugin before the engine takes its writer, so a
// cold plugin load costs the caller time while other writers proceed. The load
// must finish within loadDeadline, and the connection lives on ctx until
// release. A load that misses the deadline, while ctx is live, returns a
// world.PluginUnavailableError, and so does a load that fails; a pinned build
// proven to have no executable for this device returns a
// world.PluginExecutableMissingError. No transaction is open yet, so nothing
// was sent to apply.
func (o *bridgeOperation) PrepareApply(ctx context.Context) (func(), error) {
	// Connect on a context the deadline cancels only while the load runs.
	loadCtx, cancel := context.WithCancelCause(ctx)
	errLoadDeadline := errors.Errorf("load did not finish within %v", o.loadDeadline)
	deadline := time.AfterFunc(o.loadDeadline, func() { cancel(errLoadDeadline) })
	resources, err := o.connectPlugin(loadCtx)
	if !deadline.Stop() && err == nil {
		// The deadline fired as the load finished and ended its context.
		resources.Release()
		err = errLoadDeadline
	}
	if err != nil {
		// The deadline decides first, then the caller's own cancellation.
		timedOut := errors.Is(context.Cause(loadCtx), errLoadDeadline)
		cancel(nil)
		if timedOut {
			return nil, o.unavailable(errLoadDeadline)
		}
		return nil, err
	}
	o.prepared = resources
	return func() {
		o.prepared = nil
		resources.Release()
		cancel(nil)
	}, nil
}

// acquirePlugin returns the connection PrepareApply made, or connects on ctx
// when the operation was not prepared, as in a replay or inside a transaction
// the caller holds open. The writer is already held then, so ctx must carry
// the operation's deadline. Call release after the apply.
func (o *bridgeOperation) acquirePlugin(ctx context.Context) (*s4wave_plugin.PluginResources, func(), error) {
	if o.prepared != nil {
		return o.prepared, func() {}, nil
	}
	resources, err := o.connectPlugin(ctx)
	if err != nil {
		return nil, nil, err
	}
	return resources, resources.Release, nil
}

// withDeadline returns ctx bounded by the deadline of the started
// world.WriteBudget ctx carries, or by pluginOpDeadline without one, with a
// world.PluginOpTimeoutError for this operation as the cause when it expires.
func (o *bridgeOperation) withDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := &world.PluginOpTimeoutError{
		PluginID:        o.reg.GetPluginId(),
		OperationTypeID: o.opTypeID,
		Deadline:        pluginOpDeadline,
	}
	if budget := world.GetWriteBudget(ctx); budget != nil {
		if deadline, ok := budget.Deadline(); ok {
			timeout.Deadline = budget.GetLength()
			return context.WithDeadlineCause(ctx, deadline, timeout)
		}
	}
	return context.WithTimeoutCause(ctx, pluginOpDeadline, timeout)
}

// callErr classifies an error from reaching, attaching to or calling the
// plugin's handler; sent reports that the handler was already asked to apply.
//
// A refusal comes only from the handler's typed reply or the guard; any other
// failure to reach or run the handler is a wait, so every replica decides a
// position alike. A call into a worker loses its error's type, so an error
// that arrives as ctx ends counts as the ending's: the
// world.PluginOpTimeoutError when the deadline from withDeadline ended it,
// and err wrapping ctx's error otherwise. While ctx is live, a cancellation or
// a typed load error from connectPlugin returns as is, and every other error
// returns as a world.PluginUnavailableError.
func (o *bridgeOperation) callErr(ctx context.Context, err error, sent bool) error {
	// Nothing failed.
	if err == nil {
		return nil
	}

	// Count an error that arrives as ctx ends as the ending's.
	if ctx.Err() != nil {
		var timeout *world.PluginOpTimeoutError
		if errors.As(context.Cause(ctx), &timeout) {
			return timeout
		}
		return fmt.Errorf("%w: %w", err, ctx.Err())
	}

	// Keep a typed load error or a cancellation; anything else is a wait.
	var unavailable *world.PluginUnavailableError
	var missing *world.PluginExecutableMissingError
	if errors.As(err, &unavailable) || errors.As(err, &missing) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return &world.PluginUnavailableError{
		PluginID:        o.reg.GetPluginId(),
		OperationTypeID: o.opTypeID,
		Sent:            sent,
		Err:             err,
	}
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
		return true, o.callErr(ctx, err, false)
	}
	if msg := resp.GetError(); msg != "" {
		return false, errors.New(msg)
	}
	return false, nil
}

// connectPlugin connects to the plugin: to the registering instance, or to
// the executable at the pinned manifest root. A pinned build proven to have no
// executable for this device returns a world.PluginExecutableMissingError,
// which is final here. Any other failure while ctx is live returns a
// world.PluginUnavailableError, which may pass; a failure after ctx ended
// returns as is.
func (o *bridgeOperation) connectPlugin(ctx context.Context) (*s4wave_plugin.PluginResources, error) {
	// Connect to the pinned executable, or to the registering instance.
	var resources *s4wave_plugin.PluginResources
	var err error
	if o.manifestRoot != "" {
		resources, err = s4wave_plugin.ConnectPluginResourcesAtManifest(ctx, o.b, o.reg.GetPluginId(), o.manifestRoot)
	} else {
		resources, err = s4wave_plugin.ConnectPluginInstanceResources(ctx, o.b, o.reg.GetPluginId(), o.instanceKey)
	}
	if err == nil {
		return resources, nil
	}

	// Report a proven missing build as final, naming the root in the log.
	var missing *bldr_plugin.PinnedManifestMissingError
	if errors.As(err, &missing) {
		o.le.WithError(err).
			WithField("plugin-id", o.reg.GetPluginId()).
			WithField("operation-type-id", o.opTypeID).
			WithField("manifest-root", o.manifestRoot).
			Warn("pinned plugin has no executable for this device")
		return nil, &world.PluginExecutableMissingError{
			PluginID:        o.reg.GetPluginId(),
			OperationTypeID: o.opTypeID,
			ManifestRoot:    o.manifestRoot,
		}
	}

	// Pass a cancellation through as itself; report any other failure, even
	// one that arrives as ctx ends, as one that may pass, so a replay waits
	// instead of recording it.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	return nil, o.unavailable(err)
}

// unavailable returns the world.PluginUnavailableError for a load failure.
func (o *bridgeOperation) unavailable(err error) error {
	return &world.PluginUnavailableError{
		PluginID:        o.reg.GetPluginId(),
		OperationTypeID: o.opTypeID,
		Err:             err,
	}
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
var _ world.PreparedOperation = (*bridgeOperation)(nil)

// _ is a type assertion
var _ controller.Controller = (*WorldOpRegistryBridgeController)(nil)
