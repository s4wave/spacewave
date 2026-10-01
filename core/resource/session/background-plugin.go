package resource_session

import (
	"context"

	"github.com/pkg/errors"
	resource_space "github.com/s4wave/spacewave/core/resource/space"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// SetBackgroundPlugin records, suspends, or withdraws the user's confirmation
// that a plugin of a Space runs in the background while this Session runs.
func (r *SessionResource) SetBackgroundPlugin(
	ctx context.Context,
	req *s4wave_session.SetBackgroundPluginRequest,
) (*s4wave_session.SetBackgroundPluginResponse, error) {
	// Require both identifiers.
	spaceID, pluginID := req.GetSpaceId(), req.GetPluginId()
	if spaceID == "" {
		return nil, errors.New("space_id is required")
	}
	if pluginID == "" {
		return nil, errors.New("plugin_id is required")
	}
	if req.GetSuspended() && !req.GetEnabled() {
		return nil, errors.New("suspended requires enabled")
	}

	// Confirm only a plugin whose manifest in the Space declares background.
	if req.GetEnabled() {
		spaceResource, release, err := r.MountSpace(ctx, spaceID)
		if err != nil {
			return nil, errors.Wrap(err, "mount space")
		}
		plugin, err := spaceResource.LookupAvailablePlugin(ctx, pluginID)
		release()
		if err != nil {
			return nil, errors.Wrap(err, "look up plugin")
		}
		if plugin == nil {
			return nil, errors.Errorf("plugin %s is not available in space %s", pluginID, spaceID)
		}
		if !plugin.GetBackground() {
			return nil, errors.Errorf("plugin %s does not declare background", pluginID)
		}
	}

	// Look up the Session metadata owner.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, r.b, "", false, nil)
	if err != nil {
		return nil, errors.Wrap(err, "lookup session controller")
	}
	defer sessionCtrlRef.Release()

	// Update the choice on the current metadata.
	ref := r.session.GetSessionRef()
	meta, err := session.LookupSessionMetadata(ctx, sessionCtrl, ref)
	if err != nil {
		return nil, errors.Wrap(err, "lookup session metadata")
	}
	if meta == nil {
		meta = &session.SessionMetadata{}
	}
	var bp *session.BackgroundPlugin
	if req.GetEnabled() {
		bp = &session.BackgroundPlugin{
			SpaceId:   spaceID,
			PluginId:  pluginID,
			Suspended: req.GetSuspended(),
		}
	}
	meta.SetBackgroundPlugin(spaceID, pluginID, bp)

	// Persist the metadata, which wakes the background keeper.
	if err := sessionCtrl.UpdateSessionMetadata(ctx, ref, meta); err != nil {
		return nil, errors.Wrap(err, "update session metadata")
	}
	return &s4wave_session.SetBackgroundPluginResponse{}, nil
}

// MountSpace mounts the body of a Space in this Session's list and returns a
// SpaceResource configured like one an application mounts, so the Space plugin
// runtimes they acquire are shared. Call release when done with the resource.
func (r *SessionResource) MountSpace(
	ctx context.Context,
	spaceID string,
) (*resource_space.SpaceResource, func(), error) {
	// Find the Space in the Session's SharedObject list.
	soFeature, err := sobject.GetSharedObjectProviderAccountFeature(ctx, r.session.GetProviderAccount())
	if err != nil {
		return nil, nil, err
	}

	// Look up the Space's list entry, refreshing the list if it is missing.
	soListCtr, relSoListCtr, err := soFeature.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer relSoListCtr()
	soListEntry, err := lookupSharedObjectListEntry(ctx, soFeature, soListCtr, spaceID)
	if err != nil {
		return nil, nil, err
	}
	if soListEntry == nil {
		return nil, nil, sobject.ErrSharedObjectNotFound
	}

	// Build the SharedObject reference in the Session's provider account.
	soProviderResourceRef := r.session.GetSessionRef().GetProviderResourceRef().CloneVT()
	soProviderResourceRef.Id = spaceID
	soRef := &sobject.SharedObjectRef{
		ProviderResourceRef: soProviderResourceRef,
		BlockStoreId:        soListEntry.GetRef().GetBlockStoreId(),
	}
	if err := soRef.Validate(); err != nil {
		return nil, nil, err
	}

	// Mount the SharedObject, keeping it until release.
	so, soMountRef, err := sobject.ExMountSharedObject(ctx, r.session.GetBus(), soRef, false, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "mount shared object")
	}

	// Mount the Space body on the SharedObject.
	bodyType := soListEntry.GetMeta().GetBodyType()
	mountedSpace, bodyMountRef, err := sobject.ExMountSharedObjectBodyWithSource[space.SpaceSharedObjectBody](
		ctx,
		so.GetBus(),
		soRef,
		bodyType,
		so,
		true,
		nil,
	)
	if err != nil {
		soMountRef.Release()
		return nil, nil, errors.Wrap(err, "mount space body")
	}
	if mountedSpace == nil {
		soMountRef.Release()
		return nil, nil, errors.Wrap(sobject.ErrUnsupportedBodyType, bodyType)
	}

	// Build the SpaceResource with the application's settings.
	body := mountedSpace.GetSharedObjectBody()
	spaceResource := resource_space.NewSpaceResourceWithSessionPeerIDAndHostPluginID(
		r.le,
		r.b,
		body,
		resource_space.MountedBodySessionPeerID(body, r.session.GetPeerId().String()),
		r.hostPluginID,
	)
	spaceResource.SetAppPluginIDs(r.appPluginIDs)
	spaceResource.SetBindingRegistry(r.bindingRegistry)

	// Release the body before the SharedObject.
	release := func() {
		bodyMountRef.Release()
		soMountRef.Release()
	}
	return spaceResource, release, nil
}
