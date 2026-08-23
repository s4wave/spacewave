package resource_world

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/keyed"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
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
	le                 *logrus.Entry
	b                  bus.Bus
	ws                 world.WorldState
	engine             world.Engine
	sessionPeerID      peer.ID
	sessionPeerIDBound bool
	lifecycleCtx       context.Context
	lifecycleCancel    context.CancelFunc
	closed             atomic.Bool
	objects            *keyed.KeyedRefCount[typedObjectResourceKey, *typedObjectHandle]
}

// NewTypedObjectResource creates a new TypedObjectResource.
func NewTypedObjectResource(le *logrus.Entry, b bus.Bus, ws world.WorldState, engine world.Engine) *TypedObjectResource {
	return newTypedObjectResourceWithContextAndSessionPeerID(context.Background(), le, b, ws, engine, peer.ID(""), false)
}

// NewTypedObjectResourceWithContext creates a TypedObjectResource with a parent lifecycle context.
func NewTypedObjectResourceWithContext(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	ws world.WorldState,
	engine world.Engine,
) *TypedObjectResource {
	return newTypedObjectResourceWithContextAndSessionPeerID(ctx, le, b, ws, engine, peer.ID(""), false)
}

func newTypedObjectResourceWithSessionPeerID(
	le *logrus.Entry,
	b bus.Bus,
	ws world.WorldState,
	engine world.Engine,
	sessionPeerID peer.ID,
	sessionPeerIDBound bool,
) *TypedObjectResource {
	return newTypedObjectResourceWithContextAndSessionPeerID(
		context.Background(),
		le,
		b,
		ws,
		engine,
		sessionPeerID,
		sessionPeerIDBound,
	)
}

func newTypedObjectResourceWithContextAndSessionPeerID(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	ws world.WorldState,
	engine world.Engine,
	sessionPeerID peer.ID,
	sessionPeerIDBound bool,
) *TypedObjectResource {
	// Normalize the parent context and create resource lifecycle state.
	if ctx == nil {
		ctx = context.Background()
	}

	// Construct the typed-object resource.
	lifecycleCtx, lifecycleCancel := context.WithCancel(ctx)
	r := &TypedObjectResource{
		le:                 le,
		b:                  b,
		ws:                 ws,
		engine:             engine,
		lifecycleCtx:       lifecycleCtx,
		lifecycleCancel:    lifecycleCancel,
		sessionPeerID:      sessionPeerID,
		sessionPeerIDBound: sessionPeerIDBound,
	}

	// Initialize keyed typed-object handles.
	r.objects = keyed.NewKeyedRefCount(
		r.buildTypedObjectHandle,
		keyed.WithExitLoggerWithNameFn[typedObjectResourceKey, *typedObjectHandle](le, typedObjectResourceKey.String),
	)

	// Start handle tracking under the lifecycle context.
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
	if !r.closed.CompareAndSwap(false, true) {
		return
	}
	for _, keyedObject := range r.objects.GetKeysWithData() {
		if keyedObject.Data != nil {
			keyedObject.Data.close()
		}
	}
	r.objects.ClearContext()
	r.lifecycleCancel()
}

// AccessTypedObject looks up an object, determines its type, and returns a typed resource.
// Handles special prefixes:
//   - plugin-dist/{plugin-id}: accesses the plugin's distribution filesystem
//   - plugin-assets/{plugin-id}: accesses the plugin's assets filesystem
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

	// Select the world state used for this access.
	ws := r.ws
	if r.engine != nil && r.ws.GetReadOnly() {
		ws = world.NewEngineWorldState(r.engine, true)
	}

	// Verify that the object exists.
	_, found, err := ws.GetObject(ctx, objectKey)
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
	key := typedObjectResourceKey{
		typeID:        typeID,
		objectKey:     objectKey,
		readOnly:      ws.GetReadOnly(),
		sessionPeerID: sessionPeerID,
		engineID:      objecttype.EngineIDFromContext(ctx),
	}

	// Acquire a shared typed-object handle.
	ref, handle, _ := r.objects.AddKeyRef(key)
	if handle == nil {
		ref.Release()
		return nil, world_types.ErrUnknownObjectType
	}
	if handle.err != nil {
		ref.Release()
		return nil, handle.err
	}

	// Register the typed-object resource and its release callback.
	id, err := resourceCtx.AddResource(handle.invoker, ref.Release)
	if err != nil {
		ref.Release()
		return nil, err
	}

	return &s4wave_world.AccessTypedObjectResponse{
		ResourceId: id,
		TypeId:     typeID,
	}, nil
}

type typedObjectResourceKey struct {
	typeID        string
	objectKey     string
	readOnly      bool
	sessionPeerID peer.ID
	engineID      string
}

func (k typedObjectResourceKey) String() string {
	readOnly := "false"
	if k.readOnly {
		readOnly = "true"
	}

	name := "typed-object type=" + k.typeID + " object=" + k.objectKey + " readOnly=" + readOnly
	if k.sessionPeerID != "" {
		name += " sessionPeerID=" + k.sessionPeerID.String()
	}
	if k.engineID != "" {
		name += " engineID=" + k.engineID
	}
	return name
}

type typedObjectHandle struct {
	invoker srpc.Invoker
	cleanup func()
	closed  atomic.Bool
	err     error
}

func (h *typedObjectHandle) close() {
	if !h.closed.CompareAndSwap(false, true) {
		return
	}
	if h.cleanup != nil {
		h.cleanup()
	}
}

func (r *TypedObjectResource) buildTypedObjectHandle(key typedObjectResourceKey) (keyed.Routine, *typedObjectHandle) {
	ctx := r.lifecycleCtx
	if key.sessionPeerID != "" {
		ctx = objecttype.WithSessionPeerID(ctx, key.sessionPeerID)
	}
	if key.engineID != "" {
		ctx = objecttype.WithEngineID(ctx, key.engineID)
	}

	ws := r.ws
	if r.engine != nil && !key.readOnly && r.ws.GetReadOnly() {
		ws = world.NewEngineWorldState(r.engine, true)
	}

	objType, ref, err := objecttype.ExLookupObjectType(ctx, r.b, key.typeID)
	if err != nil {
		return nil, &typedObjectHandle{err: err}
	}
	if objType == nil {
		return nil, &typedObjectHandle{err: world_types.ErrUnknownObjectType}
	}
	defer ref.Release()

	invoker, cleanup, err := objType.GetFactory()(ctx, r.le, r.b, r.engine, ws, key.objectKey)
	if err != nil {
		return nil, &typedObjectHandle{err: err}
	}
	if cleanup == nil {
		cleanup = func() {}
	}

	handle := &typedObjectHandle{
		invoker: invoker,
		cleanup: cleanup,
	}
	routine := func(ctx context.Context) error {
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
	// Use the AccessUnixFS directive to get an FSHandle from the plugin host
	// returnIfIdle=false: wait for a resolver, valDisposeCb=nil
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

	// Get the FSHandle from the access function
	fsHandle, handleCleanup, err := accessFunc(ctx, nil)
	if err != nil {
		ref.Release()
		return nil, err
	}

	// Create the FSHandle resource which mirrors hydra/unixfs.FSHandle
	resource := resource_unixfs.NewFSHandleResource(fsHandle)

	cleanup := func() {
		handleCleanup()
		ref.Release()
	}

	// Register the typed resource
	id, err := resourceCtx.AddResourceValue(resource.GetMux(), resource, cleanup)
	if err != nil {
		cleanup()
		return nil, err
	}

	return &s4wave_world.AccessTypedObjectResponse{
		ResourceId: id,
		TypeId:     s4wave_unixfs_world.UnixFSTypeID,
	}, nil
}

// _ is a type assertion
var _ s4wave_world.SRPCTypedObjectResourceServiceServer = (*TypedObjectResource)(nil)
