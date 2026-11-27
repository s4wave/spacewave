package resource_session

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
)

// lookupMountSharedObjectListEntry uses the current catalog immediately, then
// resolves only the requested descriptor. Only unsupported providers refresh.
func lookupMountSharedObjectListEntry(
	ctx context.Context,
	feature sobject.SharedObjectProvider,
	list ccontainer.Watchable[*sobject.SharedObjectList],
	id string,
) (*sobject.SharedObjectListEntry, error) {
	// Keep cached and offline catalog hits entirely local.
	for _, entry := range list.GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == id {
			return entry, nil
		}
	}

	// A supported route's refusal or transport failure keeps its own meaning.
	entry, err := feature.LookupSharedObject(ctx, id)
	if !errors.Is(err, sobject.ErrSharedObjectLookupUnsupported) {
		return entry, err
	}
	return lookupSharedObjectListEntry(ctx, feature, list, id)
}
