package s4wave_mount

import (
	"context"
	"slices"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/world"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	sdk_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// Object is a mounted chain from a Session to one typed World object. The
// fields stay valid until Release.
type Object struct {
	// Session is the mounted Session.
	Session *s4wave_session.Session
	// SpaceID is the resolved Space shared object ID.
	SpaceID string
	// Space serves the mounted Space body.
	Space s4wave_space.SRPCSpaceResourceServiceClient
	// Engine is the Space's World engine.
	Engine *sdk_engine.SDKEngine
	// ObjectKey is the resolved typed object key.
	ObjectKey string
	// typedRef retains the typed object until Release.
	typedRef resource_client.ResourceRef
	// TypedClient reaches the typed object's services.
	TypedClient srpc.Client

	// rels releases the chain in acquisition order.
	rels []func()
}

// MountObject mounts the Session, Space, World engine and typed object for t.
// On failure it releases what it mounted.
func MountObject(ctx context.Context, res *resource_client.Client, root *s4wave_root.Root, t Target) (*Object, error) {
	// Track each acquired step so a failure unwinds the chain.
	obj := &Object{}
	fail := func(err error) (*Object, error) {
		obj.Release()
		return nil, err
	}

	// Mount the session.
	sess, err := Session(ctx, res, root, t.SessionIdx)
	if err != nil {
		return nil, err
	}
	obj.Session = sess
	obj.rels = append(obj.rels, sess.Release)

	// Resolve the Space, defaulting to the first one the Session lists.
	spaces, err := ListSpaces(ctx, sess)
	if err != nil {
		return fail(err)
	}
	if t.Space == "" && len(spaces) != 0 {
		obj.SpaceID = spaceEntryID(spaces[0])
	} else if obj.SpaceID, err = ResolveSpaceIDFromList(t.Space, spaces); err != nil {
		return fail(err)
	}

	// Mount the space.
	spaceSvc, spaceRelease, err := Space(ctx, res, sess, obj.SpaceID)
	if err != nil {
		return fail(err)
	}
	obj.Space = spaceSvc
	obj.rels = append(obj.rels, spaceRelease)

	// Access the world engine.
	engine, err := WorldEngine(ctx, res, spaceSvc)
	if err != nil {
		return fail(err)
	}
	obj.Engine = engine
	obj.rels = append(obj.rels, engine.Release)

	// Resolve the object key.
	obj.ObjectKey = t.ObjectKey
	if t.ResolveObjectKey != nil {
		obj.ObjectKey, err = t.ResolveObjectKey(ctx, engine)
		if err != nil {
			return fail(err)
		}
	}

	// Access the typed object.
	typedRef, typedClient, err := TypedObject(ctx, res, engine, obj.ObjectKey)
	if err != nil {
		return fail(errors.Wrap(err, "access typed object for "+obj.ObjectKey))
	}
	obj.typedRef, obj.TypedClient = typedRef, typedClient
	obj.rels = append(obj.rels, typedRef.Release)
	return obj, nil
}

// Release unwinds the chain in reverse acquisition order.
func (o *Object) Release() {
	for _, rel := range slices.Backward(o.rels) {
		rel()
	}
	o.rels = nil
}

// Done closes when the typed object is released, including on disconnection.
func (o *Object) Done() <-chan struct{} {
	return o.typedRef.Done()
}

// NewTransaction opens a transaction in the mounted object's World. The caller
// discards it after use, before releasing the Object.
func (o *Object) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	return o.Engine.NewTransaction(ctx, write)
}
