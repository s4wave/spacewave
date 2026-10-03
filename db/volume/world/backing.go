package volume_world

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

const (
	// ObjectTypeID identifies a Volume with an explicit transactional KV backing.
	ObjectTypeID = "hydra/volume"
	// BackingPredicate links a Volume object to the KV object it retains.
	BackingPredicate = "volume-backing"
	// KVObjectTypeID identifies the existing transactional KV object format.
	KVObjectTypeID = "kv/store"
)

// MarshalBlock encodes the Volume's backing selection.
func (b *Backing) MarshalBlock() ([]byte, error) { return b.MarshalVT() }

// UnmarshalBlock decodes the Volume's backing selection.
func (b *Backing) UnmarshalBlock(data []byte) error { return b.UnmarshalVT(data) }

// LoadBacking reads a typed Volume's backing selection.
func LoadBacking(ctx context.Context, ws world.WorldState, key string) (*Backing, error) {
	// Validate the Volume type before reading its backing selection.
	if err := world_types.CheckObjectType(ctx, ws, key, ObjectTypeID); err != nil {
		return nil, err
	}

	// Load the Volume backing and validate its referenced KV object.
	backing, err := world.LookupObjectBody[*Backing](ctx, ws, key, func() block.Block { return &Backing{} })
	if err != nil {
		return nil, err
	}
	if backing.GetKvObjectKey() == "" {
		return nil, world.ErrEmptyObjectKey
	}
	if err := world_types.CheckObjectType(ctx, ws, backing.GetKvObjectKey(), KVObjectTypeID); err != nil {
		return nil, err
	}
	return backing, nil
}
