package sobject

import (
	"context"
	"sync"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// ErrNotListedParticipant refuses a peer that no listed object admits as a
// reader.
var ErrNotListedParticipant = errors.New("peer is not a participant of a listed shared object")

// MountListedObject mounts one listed object, as a provider account's
// MountSharedObject does.
type MountListedObject func(ctx context.Context, ref *SharedObjectRef, released func()) (SharedObject, func(), error)

// mountListedHost mounts the object ref names and returns its host, or a nil
// host when the object holds no state here.
func mountListedHost(ctx context.Context, mount MountListedObject, ref *SharedObjectRef) (*SOHost, func(), error) {
	// Mount the object and keep it only when it holds a host here.
	so, release, err := mount(ctx, ref, nil)
	if err != nil {
		return nil, nil, err
	}
	holder, ok := so.(interface{ GetSOHost() *SOHost })
	if !ok {
		release()
		return nil, nil, nil
	}
	return holder.GetSOHost(), release, nil
}

// AuthorizeListedParticipant admits remote when the current config of an
// object in list names it with a role that reads state. It asks each object's
// host rather than a copy of its participants. An object that fails to mount
// admits nobody.
func AuthorizeListedParticipant(ctx context.Context, list *SharedObjectList, mount MountListedObject, remote peer.ID) error {
	id := remote.String()
	for _, entry := range list.GetSharedObjects() {
		host, release, err := mountListedHost(ctx, mount, entry.GetRef())
		if err != nil || host == nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		state, err := host.GetHostState(ctx)
		release()
		if err == nil && CanReadState(state.GetConfig().GetParticipantRole(id)) {
			return nil
		}
	}
	return ErrNotListedParticipant
}

// WatchListedParticipants calls changed after each change that may alter what
// AuthorizeListedParticipant answers: a change of the list, or of a listed
// object's config. It holds each listed object mounted until the list changes
// or ctx ends, and returns when ctx ends.
func WatchListedParticipants(ctx context.Context, lists ccontainer.Watchable[*SharedObjectList], mount MountListedObject, changed func()) error {
	for {
		// Watch the config of each object the current list names.
		list := lists.GetValue()
		watchCtx, cancel := context.WithCancel(ctx)
		var wg sync.WaitGroup
		for _, entry := range list.GetSharedObjects() {
			host, release, err := mountListedHost(watchCtx, mount, entry.GetRef())
			if err != nil || host == nil {
				continue
			}
			wg.Go(func() {
				defer release()
				watchHostConfig(watchCtx, host, changed)
			})
		}

		// Restart on a list change, which may drop an object's members.
		_, err := lists.WaitValueChange(ctx, list, nil)
		cancel()
		wg.Wait()
		if err != nil {
			return err
		}
		changed()
	}
}

// watchHostConfig calls changed after each change of host's config until ctx
// ends. A state change that keeps the config, such as a new operation, is
// not reported.
func watchHostConfig(ctx context.Context, host *SOHost, changed func()) {
	// Follow the host's state from the value it holds now.
	states, release, err := host.GetSOStateCtr(ctx, nil)
	if err != nil {
		return
	}
	defer release()
	state := states.GetValue()

	// Report each change of the config.
	for {
		next, err := states.WaitValueChange(ctx, state, nil)
		if err != nil {
			return
		}
		if !next.GetConfig().EqualVT(state.GetConfig()) {
			changed()
		}
		state = next
	}
}
