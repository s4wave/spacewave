package resource_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_unixfs "github.com/s4wave/spacewave/core/resource/unixfs"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_unixfs_world "github.com/s4wave/spacewave/sdk/unixfs/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// TypedObjectResource implements TypedObjectResourceService.
// It provides access to typed resources from world objects.
type TypedObjectResource struct {
	le     *logrus.Entry
	b      bus.Bus
	ws     world.WorldState
	engine world.Engine
}

// NewTypedObjectResource creates a new TypedObjectResource.
func NewTypedObjectResource(le *logrus.Entry, b bus.Bus, ws world.WorldState, engine world.Engine) *TypedObjectResource {
	return &TypedObjectResource{le: le, b: b, ws: ws, engine: engine}
}

// RegisterTypedObjectResource registers the TypedObjectResourceService on a mux.
func RegisterTypedObjectResource(mux srpc.Mux, le *logrus.Entry, b bus.Bus, ws world.WorldState, engine world.Engine) {
	r := NewTypedObjectResource(le, b, ws, engine)
	_ = s4wave_world.SRPCRegisterTypedObjectResourceService(mux, r)
}

// AccessTypedObject looks up an object, determines its type, and returns a typed resource.
// Handles special prefixes:
//   - plugin-dist/{plugin-id}: accesses the plugin's distribution filesystem
//   - plugin-assets/{plugin-id}: accesses the plugin's assets filesystem
func (r *TypedObjectResource) AccessTypedObject(ctx context.Context, req *s4wave_world.AccessTypedObjectRequest) (*s4wave_world.AccessTypedObjectResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	objectKey := req.GetObjectKey()
	if objectKey == "" {
		return nil, world.ErrEmptyObjectKey
	}

	// Check for plugin filesystem prefixes (plugin-dist/*, plugin-assets/*)
	_, matchedPrefix := bldr_plugin.ParsePluginUnixfsID(objectKey)
	if matchedPrefix != "" {
		return r.accessPluginUnixFS(ctx, resourceCtx, objectKey)
	}

	// Look up the object to verify it exists
	_, found, err := r.ws.GetObject(ctx, objectKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, world.ErrObjectNotFound
	}

	// Get the object type from graph quads
	typeID, err := world_types.GetObjectType(ctx, r.ws, objectKey)
	if err != nil {
		return nil, err
	}
	if typeID == "" {
		return nil, world_types.ErrUnknownObjectType
	}

	// Look up the ObjectType factory via directive
	objType, ref, err := objecttype.ExLookupObjectType(ctx, r.b, typeID)
	if err != nil {
		return nil, err
	}
	if objType == nil {
		return nil, world_types.ErrUnknownObjectType
	}
	defer ref.Release()

	// Call the factory to create the typed invoker
	factory := objType.GetFactory()
	invoker, cleanup, err := factory(ctx, r.le, r.b, r.engine, r.ws, objectKey)
	if err != nil {
		return nil, err
	}

	// Register the typed resource
	id, err := resourceCtx.AddResource(invoker, cleanup)
	if err != nil {
		cleanup()
		return nil, err
	}

	return &s4wave_world.AccessTypedObjectResponse{
		ResourceId: id,
		TypeId:     typeID,
	}, nil
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
