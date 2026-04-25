package provider_transfer

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/volume"
)

// checkpointKey is the object store key for the transfer checkpoint.
var checkpointKey = []byte("transfer/checkpoint")

// CheckpointStore provides read/write access to transfer checkpoint state.
type CheckpointStore interface {
	// LoadCheckpoint loads the checkpoint from the store.
	// Returns nil, nil if no checkpoint exists.
	LoadCheckpoint(ctx context.Context) (*TransferCheckpoint, error)
	// SaveCheckpoint persists the checkpoint to the store.
	SaveCheckpoint(ctx context.Context, cp *TransferCheckpoint) error
	// DeleteCheckpoint removes the checkpoint from the store.
	DeleteCheckpoint(ctx context.Context) error
}

// ObjectStoreCheckpoint implements CheckpointStore using an ObjectStore.
type ObjectStoreCheckpoint struct {
	store         object.ObjectStore
	b             bus.Bus
	objectStoreID string
	volumeID      string
}

// NewObjectStoreCheckpoint creates a new ObjectStoreCheckpoint with a direct store handle.
func NewObjectStoreCheckpoint(store object.ObjectStore) *ObjectStoreCheckpoint {
	return &ObjectStoreCheckpoint{store: store}
}

// NewObjectStoreCheckpointLazy creates a new ObjectStoreCheckpoint that builds
// the object store handle on first use via the bus.
func NewObjectStoreCheckpointLazy(b bus.Bus, objectStoreID, volumeID string) *ObjectStoreCheckpoint {
	return &ObjectStoreCheckpoint{
		b:             b,
		objectStoreID: objectStoreID,
		volumeID:      volumeID,
	}
}

// getStore returns the object store, building it lazily if needed.
func (c *ObjectStoreCheckpoint) getStore(ctx context.Context) (object.ObjectStore, func(), error) {
	if c.store != nil {
		return c.store, func() {}, nil
	}
	handle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, c.b, false, c.objectStoreID, c.volumeID, nil)
	if err != nil {
		return nil, nil, err
	}
	return handle.GetObjectStore(), diRef.Release, nil
}

// LoadCheckpoint loads the checkpoint from the object store.
func (c *ObjectStoreCheckpoint) LoadCheckpoint(ctx context.Context) (*TransferCheckpoint, error) {
	store, rel, err := c.getStore(ctx)
	if err != nil {
		return nil, err
	}
	defer rel()

	otx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer otx.Discard()

	data, found, err := otx.Get(ctx, checkpointKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	cp := &TransferCheckpoint{}
	if err := cp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal checkpoint")
	}
	return cp, nil
}

// SaveCheckpoint persists the checkpoint to the object store.
func (c *ObjectStoreCheckpoint) SaveCheckpoint(ctx context.Context, cp *TransferCheckpoint) error {
	data, err := cp.MarshalVT()
	if err != nil {
		return err
	}

	store, rel, err := c.getStore(ctx)
	if err != nil {
		return err
	}
	defer rel()

	otx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer otx.Discard()

	if err := otx.Set(ctx, checkpointKey, data); err != nil {
		return err
	}
	return otx.Commit(ctx)
}

// DeleteCheckpoint removes the checkpoint from the object store.
func (c *ObjectStoreCheckpoint) DeleteCheckpoint(ctx context.Context) error {
	store, rel, err := c.getStore(ctx)
	if err != nil {
		return err
	}
	defer rel()

	otx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer otx.Discard()

	if err := otx.Delete(ctx, checkpointKey); err != nil {
		return err
	}
	return otx.Commit(ctx)
}

// _ is a type assertion
var _ CheckpointStore = (*ObjectStoreCheckpoint)(nil)
