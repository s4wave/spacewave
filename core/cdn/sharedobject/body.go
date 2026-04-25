package cdn_sharedobject

import (
	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/world"
)

// CdnProviderID is the synthetic provider id used in the SharedObjectRef
// surfaced by a CdnSpaceBody. It lets call sites detect CDN-origin mounts
// without depending on the well-known CdnSpaceID string.
const CdnProviderID = "cdn"

// CdnSpaceBody adapts a CdnSharedObject and its read-only WorldEngine to
// the space.SpaceSharedObjectBody interface so MountSharedObjectBody can
// return a resource_space.SpaceResource for the CDN Space.
type CdnSpaceBody struct {
	so     *CdnSharedObject
	we     *WorldEngine
	ref    *sobject.SharedObjectRef
	engine string
	bucket string
}

// NewCdnSpaceBody constructs a SpaceSharedObjectBody wrapping the CDN
// SharedObject and its WorldEngine. The caller retains ownership of both
// and must call WorldEngine.Release when done.
//
// The synthesized SharedObjectRef carries a CdnProviderID provider id and a
// provider account id equal to the CDN Space ULID, so downstream code that
// branches on provider id (e.g. mailbox access) can recognize CDN mounts
// without depending on the well-known Space ID.
func NewCdnSpaceBody(so *CdnSharedObject, we *WorldEngine) *CdnSpaceBody {
	soID := so.GetSharedObjectID()
	bucketID := so.GetBlockStore().GetID()
	ref := &sobject.SharedObjectRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                soID,
			ProviderId:        CdnProviderID,
			ProviderAccountId: soID,
		},
		BlockStoreId: bucketID,
	}
	return &CdnSpaceBody{
		so:     so,
		we:     we,
		ref:    ref,
		engine: "cdn/" + soID,
		bucket: bucketID,
	}
}

// GetWorldEngine returns the read-only world engine for the CDN Space.
func (b *CdnSpaceBody) GetWorldEngine() world.Engine {
	return b.we.Engine
}

// GetWorldEngineID returns the world engine id for the CDN Space. It is
// "cdn/<spaceID>" so LookupWorldOp directives dispatched against the engine
// can be resolved by the standard space-world-ops controller (which accepts
// any engine id when its config EngineId is empty).
func (b *CdnSpaceBody) GetWorldEngineID() string {
	return b.engine
}

// GetWorldEngineBucketID returns the bucket id of the CDN block store. It
// equals the CDN Space ULID verbatim, per the SharedObject -> BlockStore ID
// rule (see alpha/AGENTS.md).
func (b *CdnSpaceBody) GetWorldEngineBucketID() string {
	return b.bucket
}

// GetSharedObjectRef returns the synthesized shared object ref for the CDN
// Space. The ref has a CdnProviderID provider id; downstream code that
// branches on provider id (e.g. mailbox access) skips CDN mounts.
func (b *CdnSpaceBody) GetSharedObjectRef() *sobject.SharedObjectRef {
	return b.ref
}

// GetSharedObject returns the underlying CdnSharedObject as a generic
// sobject.SharedObject handle.
func (b *CdnSpaceBody) GetSharedObject() sobject.SharedObject {
	return b.so
}

// _ is a type assertion.
var _ space.SpaceSharedObjectBody = (*CdnSpaceBody)(nil)
