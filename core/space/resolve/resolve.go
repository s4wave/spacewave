// Package resolve provides utilities for resolving resource paths to world engines.
package space_resolve

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/world"
)

// ResolvedSpace holds the result of resolving a session + shared object to a world engine.
type ResolvedSpace struct {
	// Engine is the resolved world engine.
	Engine world.Engine
	// EngineID is the world engine identifier.
	EngineID string
	// Ref is the shared object reference.
	Ref *sobject.SharedObjectRef
}

// ResolveSpace resolves a session index and shared object ID to a world engine.
// Mounts the full chain on-demand: session controller, session, provider account,
// shared object list lookup, world engine lookup by computed engine ID.
// Returns the resolved space and a cleanup function that releases all directive references.
func ResolveSpace(
	ctx context.Context,
	b bus.Bus,
	sessionIdx uint32,
	sharedObjectID string,
) (*ResolvedSpace, func(), error) {
	var refs []directive.Reference

	cleanup := func() {
		for _, v := range slices.Backward(refs) {
			v.Release()
		}
	}

	// Step 1: Look up the session controller.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, b, "", false, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "lookup session controller")
	}
	refs = append(refs, sessionCtrlRef)

	// Step 2: Get the session by index.
	sessInfo, err := sessionCtrl.GetSessionByIdx(ctx, sessionIdx)
	if err != nil {
		cleanup()
		return nil, nil, errors.Wrap(err, "get session by index")
	}
	if sessInfo == nil {
		cleanup()
		return nil, nil, errors.Errorf("session index %d not found", sessionIdx)
	}

	// Step 3: Mount the session to access its provider account.
	sessRef := sessInfo.GetSessionRef()
	sess, sessDirectiveRef, err := session.ExMountSession(ctx, b, sessRef, false, nil)
	if err != nil {
		cleanup()
		return nil, nil, errors.Wrap(err, "mount session")
	}
	refs = append(refs, sessDirectiveRef)

	// Step 4: Access the provider account's shared object list to get BlockStoreId.
	providerAcc := sess.GetProviderAccount()
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, providerAcc)
	if err != nil {
		cleanup()
		return nil, nil, errors.Wrap(err, "get shared object provider")
	}

	soListCtr, relSoListCtr, err := soProvider.AccessSharedObjectList(ctx, nil)
	if err != nil {
		cleanup()
		return nil, nil, errors.Wrap(err, "access shared object list")
	}
	defer relSoListCtr()

	soList, err := soListCtr.WaitValue(ctx, nil)
	if err != nil {
		cleanup()
		return nil, nil, errors.Wrap(err, "wait shared object list")
	}

	soIdx := slices.IndexFunc(soList.GetSharedObjects(), func(so *sobject.SharedObjectListEntry) bool {
		return so.GetRef().GetProviderResourceRef().GetId() == sharedObjectID
	})
	if soIdx == -1 {
		cleanup()
		return nil, nil, errors.Wrap(sobject.ErrSharedObjectNotFound, sharedObjectID)
	}
	soListEntry := soList.GetSharedObjects()[soIdx]

	// Step 5: Construct the full SharedObjectRef.
	sessionProvRef := sessRef.GetProviderResourceRef()
	soRef := &sobject.SharedObjectRef{
		ProviderResourceRef: sessionProvRef.CloneVT(),
		BlockStoreId:        soListEntry.GetRef().GetBlockStoreId(),
	}
	soRef.GetProviderResourceRef().Id = sharedObjectID

	// Step 6: Compute the engine ID and look up the world engine.
	engineID := space.SpaceEngineId(soRef)
	engine, _, engineRef, err := world.ExLookupWorldEngine(ctx, b, true, engineID, nil)
	if err != nil {
		cleanup()
		return nil, nil, errors.Wrap(err, "lookup world engine")
	}
	refs = append(refs, engineRef)

	return &ResolvedSpace{
		Engine:   engine,
		EngineID: engineID,
		Ref:      soRef,
	}, cleanup, nil
}
