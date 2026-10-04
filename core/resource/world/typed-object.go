package resource_world

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_unixfs "github.com/s4wave/spacewave/core/resource/unixfs"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_unixfs_world "github.com/s4wave/spacewave/sdk/unixfs/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// TypedObjectResource implements TypedObjectResourceService.
// It provides access to typed resources from world objects.
type TypedObjectResource struct {
	// le logs Resource and controller lifecycle failures.
	le *logrus.Entry
	// b resolves World and typed handler directives.
	b bus.Bus
	// ws holds the granted World snapshot and write authority.
	ws world.WorldState
	// engine retains the granted transactional World capability.
	engine world.Engine
	// sessionPeerID is the authenticated peer selected by the granting mount.
	sessionPeerID peer.ID
	// sessionPeerIDBound prevents request context from replacing the mount peer.
	sessionPeerIDBound bool
	// engineID is the registry scope selected by the granting mount.
	engineID string
	// engineIDBound prevents caller context from replacing the mount registry scope.
	engineIDBound bool
	// lifecycleCtx retains the parent mount lifecycle for typed factories.
	lifecycleCtx context.Context
	// lifecycleCancel records mount retirement before withdrawing typed demand.
	lifecycleCancel context.CancelCauseFunc
	// retirementCause distinguishes this mount's cancellation from a child failure.
	retirementCause error
	// closed guards the effectless transition to the closed state.
	closed atomic.Bool
	// objects shares typed invokers until their last Resource reference is released.
	objects *keyed.KeyedRefCount[typedObjectResourceKey, *typedObjectHandle]
}

// NewTypedObjectResource creates typed access with trusted mount options.
func NewTypedObjectResource(le *logrus.Entry, b bus.Bus, ws world.WorldState, engine world.Engine, opts ...WorldStateResourceOption) *TypedObjectResource {
	return NewTypedObjectResourceWithContext(context.Background(), le, b, ws, engine, opts...)
}

// NewTypedObjectResourceWithContext binds typed access to the mount lifecycle and scope.
func NewTypedObjectResourceWithContext(ctx context.Context, le *logrus.Entry, b bus.Bus, ws world.WorldState, engine world.Engine, opts ...WorldStateResourceOption) *TypedObjectResource {
	// Capture trusted access options with the mount lifecycle.
	access := new(WorldStateResource)
	applyWorldStateResourceOptions(access, opts...)
	lifecycleCtx, lifecycleCancel := context.WithCancelCause(ctx)
	r := &TypedObjectResource{
		le: le, b: b, ws: ws, engine: engine,
		lifecycleCtx: lifecycleCtx, lifecycleCancel: lifecycleCancel,
		retirementCause: errors.WithStack(s4wave_world.ErrTypedObjectGrantRetired),
		sessionPeerID:   access.sessionPeerID, sessionPeerIDBound: access.sessionPeerIDBound,
		engineID: access.engineID, engineIDBound: access.engineIDBound,
	}

	// Track shared unary handles until the granting mount closes.
	r.objects = keyed.NewKeyedRefCount(
		r.buildTypedObjectHandle,
		keyed.WithExitLoggerWithNameFn[typedObjectResourceKey, *typedObjectHandle](le, typedObjectResourceKey.String),
	)
	r.objects.SetContext(lifecycleCtx, false)
	return r
}

// RegisterTypedObjectResource registers the TypedObjectResourceService on a mux.
func RegisterTypedObjectResource(mux srpc.Mux, le *logrus.Entry, b bus.Bus, ws world.WorldState, engine world.Engine) {
	r := NewTypedObjectResource(le, b, ws, engine)
	_ = s4wave_world.SRPCRegisterTypedObjectResourceService(mux, r)
}

// Close releases shared typed object handles owned by this resource mount.
func (r *TypedObjectResource) Close() {
	// Cancel factories before taking the keyed lock they hold during construction.
	if !r.closed.CompareAndSwap(false, true) {
		return
	}
	r.lifecycleCancel(r.retirementCause)

	// Release constructed handles after blocked factories have returned.
	for _, keyedObject := range r.objects.GetKeysWithData() {
		if keyedObject.Data != nil {
			keyedObject.Data.close()
		}
	}
	r.objects.ClearContext()
}

// WatchTypedObject retains scoped handler demand and revokes obsolete children.
func (r *TypedObjectResource) WatchTypedObject(req *s4wave_world.WatchTypedObjectRequest, stream s4wave_world.SRPCTypedObjectResourceService_WatchTypedObjectStream) error {
	// Bind the watch to the stream, client generation and granting mount.
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	stopMount := context.AfterFunc(r.lifecycleCtx, cancel)
	defer stopMount()
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return err
	}
	stopClient := context.AfterFunc(resourceCtx.Context(), cancel)
	defer stopClient()

	// Require an object key and preserve the trusted factory scope.
	objectKey := req.GetObjectKey()
	if objectKey == "" {
		return world.ErrEmptyObjectKey
	}
	engineID := objecttype.EngineIDFromContext(ctx)
	if r.engineIDBound {
		engineID = r.engineID
	}
	ctx = objecttype.WithEngineID(ctx, engineID)
	if r.sessionPeerIDBound {
		ctx = objecttype.WithSessionPeerID(ctx, r.sessionPeerID)
	}

	// Run the typed resource's watch against the exact granted World state.
	watch := newTypedObjectWatch(r, objectKey, engineID, resourceCtx)
	return watch.execute(ctx, cancel, stream)
}

// AccessTypedObject looks up an object, determines its type, and returns a typed resource.
// Handles special prefixes:
//   - plugin-dist/{plugin-id}: accesses the plugin's distribution filesystem
//   - plugin-assets/{plugin-id}: accesses the plugin's assets filesystem.
//
// The returned child owns its typed handle until Resource release.
// Acquisition canceled by this mount's retirement returns a typed response
// error; caller and Resource client cancellation remain RPC errors.
func (r *TypedObjectResource) AccessTypedObject(ctx context.Context, req *s4wave_world.AccessTypedObjectRequest) (*s4wave_world.AccessTypedObjectResponse, error) {
	// Acquire the caller resource context.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Validate the requested object key.
	objectKey := req.GetObjectKey()
	if objectKey == "" {
		return nil, world.ErrEmptyObjectKey
	}

	// Route plugin filesystem prefixes to their specialized resource.
	_, matchedPrefix := bldr_plugin.ParsePluginUnixfsID(objectKey)
	if matchedPrefix != "" {
		return r.accessPluginUnixFS(ctx, resourceCtx, objectKey)
	}

	// Preserve the supplied state's snapshot and write authority.
	ws := r.ws

	// Verify that the object exists.
	objectState, found, err := ws.GetObject(ctx, objectKey)
	world.ReleaseObjectState(objectState)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, world.ErrObjectNotFound
	}

	// Resolve the object's graph type.
	typeID, err := world_types.GetObjectType(ctx, ws, objectKey)
	if err != nil {
		return nil, err
	}
	if typeID == "" {
		return nil, world_types.ErrUnknownObjectType
	}

	// Bind trusted session and engine context to the handle key.
	sessionPeerID := objecttype.SessionPeerIDFromContext(ctx)
	if r.sessionPeerIDBound {
		sessionPeerID = r.sessionPeerID
	}
	engineID := objecttype.EngineIDFromContext(ctx)
	if r.engineIDBound {
		engineID = r.engineID
	}
	key := typedObjectResourceKey{
		typeID:        typeID,
		objectKey:     objectKey,
		readOnly:      ws.GetReadOnly(),
		sessionPeerID: sessionPeerID,
		engineID:      engineID,
	}

	// Acquire a shared typed-object handle.
	ref, handle, _ := r.objects.AddKeyRef(key)
	if handle == nil {
		ref.Release()
		return nil, world_types.ErrUnknownObjectType
	}

	// Dispose a removed handle even when cancellation prevents its routine from starting.
	release := func() {
		// Remove this Resource's demand from the shared keyed handle.
		ref.Release()

		// A replacement key retains a different handle and cannot own this result.
		retained, _ := r.objects.GetKey(key)
		if retained != handle {
			handle.close()
		}
	}
	if handle.err != nil {
		release()
		grantRetired := errors.Is(handle.err, r.retirementCause)
		return typedObjectAcquisitionFailure(ctx, resourceCtx, handle.err, grantRetired)
	}

	// Register the typed-object resource and its release callback.
	id, err := resourceCtx.AddResource(handle.invoker, release)
	if err != nil {
		release()
		return typedObjectAcquisitionFailure(ctx, resourceCtx, err, errors.Is(err, resource.ErrResourceNotFound))
	}

	return &s4wave_world.AccessTypedObjectResponse{
		ResourceId: id,
		TypeId:     typeID,
	}, nil
}

// typedObjectResourceKey separates unary handles by object, snapshot authority and trusted scope.
type typedObjectResourceKey struct {
	// typeID identifies the registered object type.
	typeID string
	// objectKey identifies the object granted to the handler.
	objectKey string
	// readOnly retains the World snapshot write restriction.
	readOnly bool
	// sessionPeerID is the authenticated peer selected by the granting mount.
	sessionPeerID peer.ID
	// engineID is the registry scope selected by the granting mount.
	engineID string
}

// String describes the typed handle identity for lifecycle logging.
func (k typedObjectResourceKey) String() string {
	// Format the typed handle key with its snapshot write authority.
	readOnly := "false"
	if k.readOnly {
		readOnly = "true"
	}

	// Include the trusted session and registry scope in the typed handle description.
	name := "typed-object type=" + k.typeID + " object=" + k.objectKey + " readOnly=" + readOnly
	if k.sessionPeerID != "" {
		name += " sessionPeerID=" + k.sessionPeerID.String()
	}
	if k.engineID != "" {
		name += " engineID=" + k.engineID
	}
	return name
}

// typedObjectHandle retains one registered invoker until its final Resource reference is released.
type typedObjectHandle struct {
	// invoker forwards calls to the registered typed handler.
	invoker srpc.Invoker
	// cleanup releases the registered typed handler.
	cleanup func()
	// closed guards the effectless transition to the closed state.
	closed atomic.Bool
	// err records the input or typed factory resolution error.
	err error
}

// close releases the registered invoker exactly once.
func (h *typedObjectHandle) close() {
	// Claim the invoker cleanup before releasing its factory resources.
	if !h.closed.CompareAndSwap(false, true) {
		return
	}

	// Release the factory resources when the invoker owns a cleanup callback.
	if h.cleanup != nil {
		h.cleanup()
	}
}

// buildTypedObjectHandle resolves the factory with the trusted unary handle scope.
func (r *TypedObjectResource) buildTypedObjectHandle(key typedObjectResourceKey) (keyed.Routine, *typedObjectHandle) {
	// Rebuild the trusted factory context from the typed handle key.
	ctx := r.lifecycleCtx
	if key.sessionPeerID != "" {
		ctx = objecttype.WithSessionPeerID(ctx, key.sessionPeerID)
	}
	ctx = objecttype.WithEngineID(ctx, key.engineID)

	// A typed factory receives the same state that resolved its object and type.
	ws := r.ws

	// Resolve the object type under the selected engine scope.
	objType, ref, err := objecttype.ExLookupObjectType(ctx, r.b, key.typeID)
	if err != nil {
		if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
			err = context.Cause(ctx)
		}
		return nil, &typedObjectHandle{err: err}
	}
	if objType == nil {
		return nil, &typedObjectHandle{err: world_types.ErrUnknownObjectType}
	}
	defer ref.Release()

	// Construct the typed invoker from the registered factory.
	invoker, cleanup, err := objType.GetFactory()(ctx, r.le, r.b, r.engine, ws, key.objectKey)
	if err != nil {
		if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
			err = context.Cause(ctx)
		}
		return nil, &typedObjectHandle{err: err}
	}

	// Keep canceled factory results for disposal without registering a child.
	handle := &typedObjectHandle{
		invoker: invoker,
		cleanup: cleanup,
	}
	if err := ctx.Err(); err != nil {
		handle.err = context.Cause(ctx)
		return nil, handle
	}

	// Retain the invoker until the mount or its last Resource reference is released.
	routine := func(ctx context.Context) error {
		// Release the invoker after its shared routine is canceled.
		<-ctx.Done()
		handle.close()
		return nil
	}
	return routine, handle
}

// accessPluginUnixFS accesses a plugin filesystem via the AccessUnixFS directive.
func (r *TypedObjectResource) accessPluginUnixFS(
	ctx context.Context,
	resourceCtx resource_server.ResourceClientContext,
	unixfsID string,
) (*s4wave_world.AccessTypedObjectResponse, error) {
	// Retain plugin filesystem demand until its Resource is released.
	accessFunc, ref, err := unixfs_access.ExAccessUnixFS(ctx, r.b, unixfsID, false, nil)
	if err != nil {
		return nil, err
	}
	if accessFunc == nil {
		if ref != nil {
			ref.Release()
		}
		return nil, world.ErrObjectNotFound
	}

	// Acquire the filesystem handle from the resolved plugin.
	fsHandle, handleCleanup, err := accessFunc(ctx, nil)
	if err != nil {
		ref.Release()
		return nil, err
	}

	// Serve the acquired filesystem through its Resource interface.
	fsResource := resource_unixfs.NewFSHandleResource(fsHandle)

	// Bind filesystem cleanup to the registered typed Resource.
	cleanup := func() {
		handleCleanup()
		ref.Release()
	}

	// Transfer filesystem ownership to the registered child Resource.
	id, err := resourceCtx.AddResourceValue(fsResource.GetMux(), fsResource, cleanup)
	if err != nil {
		cleanup()
		return typedObjectAcquisitionFailure(ctx, resourceCtx, err, errors.Is(err, resource.ErrResourceNotFound))
	}

	return &s4wave_world.AccessTypedObjectResponse{
		ResourceId: id,
		TypeId:     s4wave_unixfs_world.UnixFSTypeID,
	}, nil
}

// typedObjectAcquisitionFailure carries retirement proved by the granting factory
// context or registration's missing invocation parent, preserving terminal causes.
func typedObjectAcquisitionFailure(ctx context.Context, resourceCtx resource_server.ResourceClientContext, err error, grantRetired bool) (*s4wave_world.AccessTypedObjectResponse, error) {
	// Keep child failures terminal while the exact granting mount remains healthy.
	if !grantRetired {
		return nil, err
	}

	// Preserve terminal caller and Resource client cancellation.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := resourceCtx.Context().Err(); err != nil {
		return nil, err
	}

	// Carry exact mount retirement before asynchronous release controls.
	return &s4wave_world.AccessTypedObjectResponse{
		ErrorCode: s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_TYPED_OBJECT_GRANT_RETIRED,
	}, nil
}

// _ is a type assertion
var _ s4wave_world.SRPCTypedObjectResourceServiceServer = (*TypedObjectResource)(nil)
