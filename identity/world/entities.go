package identity_world

import (
	"context"
	"sort"
	"strings"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/identity"
)

const (
	// EntityPrefix is the prefix applied to domain-id/entity-id.
	EntityPrefix = "e/"
	// EntityTypeID is the type identifier for a Entity.
	EntityTypeID = "identity/entity"

	// PredEntityToDomainInfo is the predicate linking Entity to a DomainInfo.
	PredEntityToDomainInfo = quad.IRI("identity/entity-domain")
)

// NewEntityKey builds a key from a session id.
func NewEntityKey(domainID, entityID string) string {
	return strings.Join([]string{
		EntityPrefix,
		domainID, "/",
		entityID,
	}, "")
}

// NewEntityToDomainInfoQuad creates a quad linking an entity to a domain info.
func NewEntityToDomainInfoQuad(entityObjKey, domainInfoObjKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(
		entityObjKey,
		PredEntityToDomainInfo.String(),
		domainInfoObjKey,
		"",
	)
}

// FollowEntity follows & checks a reference to a Entity.
func FollowEntity(
	ctx context.Context,
	accessState world.AccessWorldStateFunc,
	entityRef *bucket.ObjectRef,
) (*identity.Entity, error) {
	var err error
	if entityRef.GetEmpty() {
		return nil, errors.New("empty entity ref")
	}
	if err := entityRef.Validate(); err != nil {
		return nil, err
	}
	var entity *identity.Entity
	_, err = world.AccessObject(ctx, accessState, entityRef, func(bcs *block.Cursor) error {
		// Confirm valid Entity object.
		var err error
		entity, err = identity.UnmarshalEntity(ctx, bcs)
		return err
	})
	if err == nil {
		err = entity.Validate()
	}
	if err != nil {
		return nil, err
	}
	return entity, nil
}

// LookupEntityOp performs the lookup operation for the Entity op types.
func LookupEntityOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	switch opTypeID {
	case EntityUpdateOpId:
		return &EntityUpdateOp{}, nil
	}
	return nil, nil
}

// LookupEntity looks up an entity with the given key.
// returns nil, nil, nil if not found.
func LookupEntity(ctx context.Context, w world.WorldState, objKey string) (*identity.Entity, world.ObjectState, error) {
	obj, objFound, err := w.GetObject(ctx, objKey)
	if err != nil {
		return nil, nil, err
	}
	if !objFound {
		return nil, nil, nil
	}
	var entity *identity.Entity
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		entity, err = identity.UnmarshalEntity(ctx, bcs)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return entity, obj, nil
}

// LookupEntities looks up a list of entities by object key.
func LookupEntities(ctx context.Context, w world.WorldState, objKeys []string) ([]*identity.Entity, error) {
	ents := make([]*identity.Entity, len(objKeys))
	var err error
	for i, objKey := range objKeys {
		ents[i], _, err = LookupEntity(ctx, w, objKey)
		if err != nil {
			return nil, err
		}
	}

	return ents, nil
}

// CollectAllEntities collects all Entity states located in the store.
// returns list of entities and object keys
func CollectAllEntities(ctx context.Context, w world.WorldState) ([]*identity.Entity, []string, error) {
	var objKeys []string
	err := world_types.IterateObjectsWithType(ctx, w, EntityTypeID, func(objKey string) (bool, error) {
		if !strings.HasPrefix(objKey, EntityPrefix) {
			return true, nil
		}
		objKeys = append(objKeys, objKey)
		return true, nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(objKeys)

	// collect entity list
	entityList, err := LookupEntities(ctx, w, objKeys)
	return entityList, objKeys, err
}

// ListEntityKeypairs lists all Keypair linked to by the given entities.
// returns list of object keys
func ListEntityKeypairs(ctx context.Context, w world.WorldState, entityKeys ...string) ([]string, error) {
	return ListObjectKeypairs(ctx, w, entityKeys...)
}

// CollectEntityKeypairs collects all Keypair linked to by the given entities.
// returns list of Keypair for each object key
func CollectEntityKeypairs(ctx context.Context, w world.WorldState, entityKeys ...string) ([]*identity.Keypair, []string, error) {
	return CollectObjectKeypairs(ctx, w, entityKeys...)
}
