// Package s4wave_mount mounts the Resource chain from a Session to a Space,
// its World engine and one typed World object. Command-line tools and other
// SDK clients compose it over an initialized Resource client.
package s4wave_mount

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	s4wave_space_core "github.com/s4wave/spacewave/core/space"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	sdk_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// Session mounts the Session at idx. The caller releases it.
func Session(ctx context.Context, res *resource_client.Client, root *s4wave_root.Root, idx uint32) (*s4wave_session.Session, error) {
	// Mount the session by index.
	resp, err := root.MountSessionByIdx(ctx, idx)
	if err != nil {
		return nil, errors.Wrap(err, "mount session")
	}
	if resp.GetNotFound() {
		return nil, errors.Errorf("no session found at index %d", idx)
	}

	// Wrap the session resource.
	sessRef := res.CreateResourceReference(resp.GetResourceId())
	sess, err := s4wave_session.NewSession(res, sessRef)
	if err != nil {
		sessRef.Release()
		return nil, errors.Wrap(err, "session resource")
	}
	return sess, nil
}

// SharedObject mounts a shared object by ID and returns its service and the
// func that releases it.
func SharedObject(ctx context.Context, res *resource_client.Client, sess *s4wave_session.Session, sharedObjectID string) (s4wave_sobject.SRPCSharedObjectResourceServiceClient, func(), error) {
	// Mount the shared object.
	soResp, err := sess.MountSharedObject(ctx, sharedObjectID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "mount shared object")
	}

	// Open the shared object resource client.
	soRef := res.CreateResourceReference(soResp.GetResourceId())
	soClient, err := soRef.GetClient()
	if err != nil {
		soRef.Release()
		return nil, nil, errors.Wrap(err, "shared object client")
	}
	return s4wave_sobject.NewSRPCSharedObjectResourceServiceClient(soClient), soRef.Release, nil
}

// Space mounts the Space body of a shared object and returns its service and
// the func that releases the body and the shared object.
func Space(ctx context.Context, res *resource_client.Client, sess *s4wave_session.Session, sharedObjectID string) (s4wave_space.SRPCSpaceResourceServiceClient, func(), error) {
	// Mount the shared object and its space body.
	soSvc, soRelease, err := SharedObject(ctx, res, sess, sharedObjectID)
	if err != nil {
		return nil, nil, err
	}
	bodyResp, err := s4wave_sobject.MountSharedObjectBody(ctx, soSvc)
	if err != nil {
		soRelease()
		return nil, nil, errors.Wrap(err, "mount shared object body")
	}

	// Open the space body resource client.
	bodyRef := res.CreateResourceReference(bodyResp.GetResourceId())
	bodyClient, err := bodyRef.GetClient()
	if err != nil {
		bodyRef.Release()
		soRelease()
		return nil, nil, errors.Wrap(err, "space body client")
	}

	// Build the space service client and the cleanup.
	release := func() {
		bodyRef.Release()
		soRelease()
	}
	return s4wave_space.NewSRPCSpaceResourceServiceClient(bodyClient), release, nil
}

// ListSpaces returns the Session's current Space list snapshot.
func ListSpaces(ctx context.Context, sess *s4wave_session.Session) ([]*s4wave_space_core.SpaceSoListEntry, error) {
	// Watch the resource list stream and receive the first snapshot.
	strm, err := sess.WatchResourcesList(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "watch resources list")
	}
	defer strm.Close()
	resp, err := strm.Recv()
	if err != nil {
		return nil, errors.Wrap(err, "recv resources list")
	}
	return resp.GetSpacesList(), nil
}

// ResolveSpaceID resolves a Space ID or display name against the Session's
// Space list. An empty space selects the sole Space.
func ResolveSpaceID(ctx context.Context, sess *s4wave_session.Session, space string) (string, error) {
	spaces, err := ListSpaces(ctx, sess)
	if err != nil {
		return "", err
	}
	return ResolveSpaceIDFromList(space, spaces)
}

// ResolveSpaceIDFromList selects an exact ID, display name, or sole Space,
// and rejects a Space the list does not hold.
func ResolveSpaceIDFromList(space string, spaces []*s4wave_space_core.SpaceSoListEntry) (string, error) {
	// Require at least one Space to choose from.
	if len(spaces) == 0 {
		return "", errors.New("no spaces found; specify --space")
	}

	// Match an exact ID first, then a display name.
	if space != "" {
		for _, sp := range spaces {
			if id := spaceEntryID(sp); id == space {
				return id, nil
			}
		}
		for _, sp := range spaces {
			if sp.GetSpaceMeta().GetName() == space {
				return spaceEntryID(sp), nil
			}
		}
		return "", errors.Errorf("space %q not found in this session; an ID can predate a reset, so prefer the Space name from spacewave space list", space)
	}

	// Without a selection, only a sole Space is unambiguous.
	if len(spaces) > 1 {
		return "", errors.New("multiple spaces found; specify --space")
	}
	return spaceEntryID(spaces[0]), nil
}

// spaceEntryID returns the shared object ID of a Space list entry.
func spaceEntryID(sp *s4wave_space_core.SpaceSoListEntry) string {
	return sp.GetEntry().GetRef().GetProviderResourceRef().GetId()
}

// WorldEngine accesses the Space's World engine. The caller releases it.
func WorldEngine(ctx context.Context, res *resource_client.Client, spaceSvc s4wave_space.SRPCSpaceResourceServiceClient) (*sdk_engine.SDKEngine, error) {
	// Access the world resource.
	worldResp, err := spaceSvc.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "access world")
	}

	// Wrap the engine resource reference.
	engineRef := res.CreateResourceReference(worldResp.GetResourceId())
	engine, err := sdk_engine.NewSDKEngine(res, engineRef)
	if err != nil {
		engineRef.Release()
		return nil, errors.Wrap(err, "create sdk engine")
	}
	return engine, nil
}

// TypedObject accesses the typed object at objectKey through a World engine.
// It returns the object's resource reference, which the caller
// releases, and the client for its services.
func TypedObject(ctx context.Context, res *resource_client.Client, engine *sdk_engine.SDKEngine, objectKey string) (resource_client.ResourceRef, srpc.Client, error) {
	return engine.TypedObject(ctx, res, objectKey)
}
