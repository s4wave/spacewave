package bstore

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// MountBlockStore is a directive to mount a block store with a provider account.
type MountBlockStore interface {
	// Directive indicates MountBlockStore is a directive.
	directive.Directive

	// MountBlockStoreRef returns the block store ref to mount.
	MountBlockStoreRef() *BlockStoreRef
}

// MountBlockStoreValue is the result type for MountBlockStore.
type MountBlockStoreValue = BlockStore

// ExMountBlockStore executes a lookup for a single provider on the bus.
//
// If returnIfIdle is set, returns when the directive becomes idle.
func ExMountBlockStore(
	ctx context.Context,
	b bus.Bus,
	ref *BlockStoreRef,
	returnIfIdle bool,
	valDisposeCb func(),
) (BlockStore, directive.Reference, error) {
	av, _, avRef, err := bus.ExecOneOffTyped[MountBlockStoreValue](
		ctx,
		b,
		NewMountBlockStore(ref),
		bus.ReturnIfIdle(returnIfIdle),
		valDisposeCb,
	)
	if err != nil {
		return nil, nil, err
	}
	if av == nil {
		avRef.Release()
		return nil, nil, nil
	}
	return av.GetValue(), avRef, nil
}

// mountBlockStore implements MountBlockStore
type mountBlockStore struct {
	ref *BlockStoreRef
}

// NewMountBlockStore constructs a new MountBlockStore directive.
func NewMountBlockStore(ref *BlockStoreRef) MountBlockStore {
	return &mountBlockStore{
		ref: ref,
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *mountBlockStore) Validate() error {
	if err := d.ref.Validate(); err != nil {
		return err
	}
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *mountBlockStore) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// MountBlockStoreRef returns the shared object id to mount.
func (d *mountBlockStore) MountBlockStoreRef() *BlockStoreRef {
	return d.ref
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *mountBlockStore) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(MountBlockStore)
	if !ok {
		return false
	}

	return d.ref.EqualVT(od.MountBlockStoreRef())
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *mountBlockStore) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *mountBlockStore) GetName() string {
	return "MountBlockStore"
}

// GetDebugVals returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *mountBlockStore) GetDebugVals() directive.DebugValues {
	return directive.DebugValues{
		"bstore-id":   []string{d.ref.GetProviderResourceRef().GetId()},
		"provider-id": []string{d.ref.GetProviderResourceRef().GetProviderId()},
		"account-id":  []string{d.ref.GetProviderResourceRef().GetProviderAccountId()},
	}
}

// _ is a type assertion
var _ MountBlockStore = ((*mountBlockStore)(nil))
