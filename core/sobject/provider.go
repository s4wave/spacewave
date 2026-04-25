package sobject

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	provider "github.com/s4wave/spacewave/core/provider"
)

// OwnerTypeAccount is the owner type for account-owned shared objects.
const OwnerTypeAccount = "account"

// OwnerTypeOrganization is the owner type for org-owned shared objects.
const OwnerTypeOrganization = "organization"

// SharedObjectProvider implements ProviderFeature_SHARED_OBJECT.
type SharedObjectProvider interface {
	provider.ProviderAccountFeature

	// CreateSharedObject creates a new shared object with the given details.
	// The ID may not necessarily be used for the shared object ID.
	// ownerType is "account" or "organization"; ownerID is the principal id
	// (account id for account-owned, org id for org-owned).
	CreateSharedObject(ctx context.Context, id string, meta *SharedObjectMeta, ownerType, ownerID string) (*SharedObjectRef, error)

	// MountSharedObject attempts to mount a SharedObject returning the object handle and a release function.
	//
	// This also mounts the block store associated with the shared object.
	//
	// note: use the MountSharedObject directive to call this.
	// usually called by the provider controller
	MountSharedObject(ctx context.Context, ref *SharedObjectRef, released func()) (SharedObject, func(), error)

	// DeleteSharedObject deletes the shared object with the given ID.
	// Removes from the shared object list, cleans up GC references,
	// and removes the bucket from the volume.
	DeleteSharedObject(ctx context.Context, id string) error

	// AccessSharedObjectList adds a reference to the list of shared objects and returns the container.
	// Returns a release function. Accepts a function that is called if the Watchable becomes invalid.
	AccessSharedObjectList(ctx context.Context, released func()) (ccontainer.Watchable[*SharedObjectList], func(), error)
}

// SharedObjectHealthProvider streams SharedObject health snapshots before body mount.
type SharedObjectHealthProvider interface {
	// AccessSharedObjectHealth adds a reference to SharedObject health by ref.
	// Returns a release function. Accepts a function that is called if the Watchable becomes invalid.
	AccessSharedObjectHealth(ctx context.Context, ref *SharedObjectRef, released func()) (ccontainer.Watchable[*SharedObjectHealth], func(), error)
}

// GetSharedObjectProviderAccountFeature returns the SharedObjectProvider for a ProviderAccount.
func GetSharedObjectProviderAccountFeature(ctx context.Context, provAcc provider.ProviderAccount) (SharedObjectProvider, error) {
	return provider.GetProviderAccountFeature[SharedObjectProvider](
		ctx,
		provAcc,
		provider.ProviderFeature_ProviderFeature_SHARED_OBJECT,
	)
}

// GetSharedObjectHealthProvider returns the optional SharedObjectHealthProvider for a ProviderAccount.
func GetSharedObjectHealthProvider(
	provAcc provider.ProviderAccount,
) (SharedObjectHealthProvider, bool) {
	feature, ok := provAcc.(SharedObjectHealthProvider)
	return feature, ok
}
