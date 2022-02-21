package identity_world

import (
	"context"
	"sort"
	"strings"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/aperturerobotics/identity"
	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/quad"
	"github.com/pkg/errors"
)

const (
	// KeypairPrefix is the prefix applied to the keypair peer id.
	KeypairPrefix = "kp/"
	// KeypairTypeID is the type identifier for a Keypair.
	KeypairTypeID = "identity/keypair"

	// PredObjectToKeypair links any object to a Keypair.
	// The meaning of the link is source-specific.
	PredObjectToKeypair = quad.IRI(KeypairTypeID + "-link")
)

// NewKeypairKey builds a key from a peer id.
func NewKeypairKey(peerIDPretty string) string {
	return KeypairPrefix + peerIDPretty
}

// NewObjectToKeypairQuad creates a quad linking any object to a Keypair.
func NewObjectToKeypairQuad(objKey, keypairObjKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(
		objKey,
		PredObjectToKeypair.String(),
		keypairObjKey,
		"",
	)
}

// FollowKeypair follows & checks a reference to a Keypair.
func FollowKeypair(
	ctx context.Context,
	accessState world.AccessWorldStateFunc,
	keypairRef *bucket.ObjectRef,
) (*identity.Keypair, error) {
	var err error
	if keypairRef.GetEmpty() {
		return nil, errors.New("empty keypair ref")
	}
	if err := keypairRef.Validate(); err != nil {
		return nil, err
	}
	var entity *identity.Keypair
	_, err = world.AccessObject(ctx, accessState, keypairRef, func(bcs *block.Cursor) error {
		// Confirm valid Keypair object.
		var err error
		entity, err = identity.UnmarshalKeypair(bcs)
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

// LookupKeypairOp performs the lookup operation for the Keypair op types.
func LookupKeypairOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	switch opTypeID {
	case KeypairUpdateOpId:
		return &KeypairUpdateOp{}, nil
	}
	return nil, nil
}

// LookupKeypair looks up an entity with the given key.
// returns nil, nil, nil if not found.
func LookupKeypair(ctx context.Context, w world.WorldState, objKey string) (*identity.Keypair, world.ObjectState, error) {
	obj, objFound, err := w.GetObject(objKey)
	if err != nil {
		return nil, nil, err
	}
	if !objFound {
		return nil, nil, nil
	}
	var entity *identity.Keypair
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		entity, err = identity.UnmarshalKeypair(bcs)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return entity, obj, nil
}

// LookupKeypairs looks up a set of keypairs.
func LookupKeypairs(ctx context.Context, w world.WorldState, objKeys []string) ([]*identity.Keypair, error) {
	kps := make([]*identity.Keypair, len(objKeys))
	for i, objKey := range objKeys {
		var err error
		kps[i], _, err = LookupKeypair(ctx, w, objKey)
		if err != nil {
			return nil, err
		}
	}
	return kps, nil
}

// CollectAllKeypairs collects all Keypair states located in the store.
// returns list of entities and object keys
func CollectAllKeypairs(ctx context.Context, w world.WorldState) ([]*identity.Keypair, []string, error) {
	var objKeys []string
	ts := world_types.NewTypesState(ctx, w)
	err := ts.IterateObjectsWithType(KeypairTypeID, func(objKey string) (bool, error) {
		if !strings.HasPrefix(objKey, KeypairPrefix) {
			return true, nil
		}
		objKeys = append(objKeys, objKey)
		return true, nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(objKeys)

	// collect list
	list, err := LookupKeypairs(ctx, w, objKeys)
	return list, objKeys, err
}

// ListKeypairEntities lists all Entity that link to the given keypairs.
// returns list of object keys
func ListKeypairEntities(ctx context.Context, w world.WorldState, keypairKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		w,
		keypairKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			return p.In(PredEntityToKeypair), nil
		},
	)
}

// CollectKeypairEntities collects all Entity linking to the keypairs.
// returns list of Entities and object keys
func CollectKeypairEntities(ctx context.Context, w world.WorldState, keypairKeys ...string) ([]*identity.Entity, error) {
	objKeys, err := ListKeypairEntities(ctx, w, keypairKeys...)
	if err != nil {
		return nil, err
	}

	return LookupEntities(ctx, w, objKeys)
}

// ListKeypairLinks collects all Object linking directly to the Keypair.
// returns list of object keys
func ListKeypairLinks(ctx context.Context, w world.WorldState, keypairKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		w,
		keypairKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			return p.In(PredObjectToKeypair), nil
		},
	)
}
