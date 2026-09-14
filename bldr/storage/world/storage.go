package storage_world

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/s4wave/spacewave/bldr/storage"
	"github.com/s4wave/spacewave/db/block"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_world "github.com/s4wave/spacewave/db/volume/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// Storage allocates named Volume objects beneath one stable World prefix.
// The caller retains the engine and its backing storage for this lifetime.
// A prefix separates names; access to the World determines authority.
type Storage struct {
	ctx      context.Context
	engine   world.Engine
	engineID string
	prefix   string
}

// NewStorage binds named Volumes to an engine already available on the bus.
func NewStorage(ctx context.Context, b bus.Bus, engineID, prefix string) (*Storage, error) {
	if engineID == "" {
		return nil, world.ErrEmptyEngineID
	}
	prefix = strings.TrimRight(prefix, "/")
	if prefix == "" {
		return nil, errors.New("world storage requires an object prefix")
	}
	return &Storage{ctx: ctx, engine: world.NewBusEngine(ctx, b, engineID), engineID: engineID, prefix: prefix}, nil
}

// GetStorageInfo returns the named storage capabilities.
func (s *Storage) GetStorageInfo() *storage.StorageInfo { return &storage.StorageInfo{} }

// AddFactories installs the World Volume implementation on the consuming bus.
func (s *Storage) AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(volume_world.NewFactory(b))
}

// VolumeKey maps an arbitrary nonempty name to one unambiguous object key.
func (s *Storage) VolumeKey(id string) (string, error) {
	if id == "" {
		return "", errors.New("world storage volume name cannot be empty")
	}
	return s.prefix + "/volumes/" + base64.RawURLEncoding.EncodeToString([]byte(id)), nil
}

// BuildVolumeConfig creates or reopens a typed Volume and its owned KV backing.
func (s *Storage) BuildVolumeConfig(id string, base *volume_controller.Config) (config.Config, error) {
	key, err := s.VolumeKey(id)
	if err != nil {
		return nil, err
	}
	backing, err := s.ensureVolume(key)
	if err != nil {
		return nil, err
	}
	return &volume_world.Config{EngineId: s.engineID, ObjectKey: backing.GetKvObjectKey(), VolumeConfig: base}, nil
}

// ensureVolume commits the descriptor, KV object, and ownership edge together.
func (s *Storage) ensureVolume(key string) (*volume_world.Backing, error) {
	ctx := s.ctx
	tx, err := s.engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	exists, err := tx.HasObject(ctx, key)
	if err != nil {
		return nil, err
	}
	if exists {
		return volume_world.LoadBacking(ctx, tx, key)
	}
	backing := &volume_world.Backing{KvObjectKey: key + "/kv"}
	kvObject, err := tx.CreateObject(ctx, backing.KvObjectKey, nil)
	world.ReleaseObjectState(kvObject)
	if err != nil {
		return nil, err
	}
	if err := world_types.SetObjectType(ctx, tx, backing.KvObjectKey, volume_world.KVObjectTypeID); err != nil {
		return nil, err
	}
	obj, _, err := world.CreateWorldObject(ctx, tx, key, func(cursor *block.Cursor) error {
		cursor.SetBlock(backing, true)
		return nil
	})
	world.ReleaseObjectState(obj)
	if err != nil {
		return nil, err
	}
	if err := world_types.SetObjectType(ctx, tx, key, volume_world.ObjectTypeID); err != nil {
		return nil, err
	}
	if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(key, volume_world.BackingPredicate, backing.KvObjectKey, "")); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return backing, nil
}

// DeleteVolume removes the closed Volume and its unshared owned KV object.
// Shared backing objects and the enclosing World's block storage remain alive.
func (s *Storage) DeleteVolume(id string) error {
	key, err := s.VolumeKey(id)
	if err != nil {
		return err
	}
	ctx := s.ctx
	tx, err := s.engine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	exists, err := tx.HasObject(ctx, key)
	if err != nil || !exists {
		return err
	}
	backing, err := volume_world.LoadBacking(ctx, tx, key)
	if err != nil {
		return err
	}
	if _, err := tx.DeleteObject(ctx, key); err != nil {
		return err
	}
	if backing.KvObjectKey == key+"/kv" {
		refs, err := tx.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("", "", backing.KvObjectKey, ""), 1)
		if err != nil {
			return err
		}
		if len(refs) == 0 {
			if _, err := tx.DeleteObject(ctx, backing.KvObjectKey); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

var _ storage.Storage = (*Storage)(nil)
