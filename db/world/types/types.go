package world_types

import (
	"context"
	"strings"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// TypesPrefix is the prefix string for all types identifiers.
const TypesPrefix = "types/"

// TypePred is the predicate linking a object to its type.
var TypePred quad.Value = quad.IRI("type")

const typeGraphLookupLimit uint32 = 1_000_000

// ObjectTypeLister lists objects by type without requiring a Cayley handle.
type ObjectTypeLister interface {
	// ListObjectsWithType lists object keys with the given type identifier.
	ListObjectsWithType(ctx context.Context, typeID string) ([]string, error)
}

// BuildTypeObjectKey returns the object key referring to the type.
func BuildTypeObjectKey(typeID string) string {
	if typeID == "" {
		return ""
	}
	return TypesPrefix + typeID
}

// BuildTypeQuadValue returns the quad value referring to the type.
func BuildTypeQuadValue(typeID string) quad.Value {
	if typeID == "" {
		return nil
	}
	return world.KeyToGraphValue(BuildTypeObjectKey(typeID))
}

// BuildTypeQuad returns a type quad for a key and type.
func BuildTypeQuad(objKey, typeID string) quad.Quad {
	subjVal := world.KeyToGraphValue(objKey)
	typeVal := BuildTypeQuadValue(typeID)
	return quad.Quad{
		Subject:   subjVal,
		Predicate: TypePred,
		Object:    typeVal,
	}
}

// LimitNodesToTypes limits the matched nodes to the given types in the Path.
func LimitNodesToTypes(path *cayley.Path, typeIDs ...string) *cayley.Path {
	typeNodes := make([]quad.Value, len(typeIDs))
	for i, typeID := range typeIDs {
		typeNodes[i] = BuildTypeQuadValue(typeID)
	}
	return path.Has(TypePred, typeNodes...)
}

// GetObjectType returns the type of a given object.
// Returns "" if the object has no type.
func GetObjectType(ctx context.Context, ws world.WorldState, key string) (string, error) {
	// Read the object type through the World metadata batch API when available.
	if batcher, ok := ws.(ObjectMetadataBatcher); ok {
		metadata, err := batcher.GetObjectMetadataBatch(ctx, []string{key})
		if err != nil || len(metadata) == 0 {
			return "", err
		}
		return metadata[0].TypeID, nil
	}

	// Find the object type key among its outgoing type quads.
	var typeKey string
	quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, TypePred.String(), "", ""), typeGraphLookupLimit)
	if err != nil {
		return "", err
	}

	// Decode the type graph values until a type object key is found.
	for _, q := range quads {
		objKey, err := world.GraphValueToKey(q.GetObj())
		if err != nil {
			return "", err
		}
		if strings.HasPrefix(objKey, TypesPrefix) {
			typeKey = objKey
			break
		}
	}

	// Report an absent type when no type object key matched.
	if len(typeKey) == 0 {
		return "", nil
	}
	return typeKey[len(TypesPrefix):], nil
}

// CheckObjectType asserts that the object key exists and has the given type.
func CheckObjectType(ctx context.Context, ws world.WorldState, key, typeID string) error {
	objType, err := GetObjectType(ctx, ws, key)
	if err != nil {
		return err
	}
	if objType != typeID {
		if objType == "" {
			return errors.Errorf("object %s: expected object to exist w/ a valid type", key)
		}
		return errors.Errorf("object %s: expected type %s but got %q", key, typeID, objType)
	}
	return err
}

// SetObjectType sets the type of a given object by writing a graph quad.
func SetObjectType(ctx context.Context, ws world.WorldState, key, typeID string) error {
	// Require object and type keys before changing the type graph.
	if key == "" || typeID == "" {
		return world.ErrEmptyObjectKey
	}

	// Create the type object if it is absent.
	if err := EnsureTypeExists(ctx, ws, typeID); err != nil {
		return err
	}

	// Replace other type quads while retaining an existing matching quad.
	nextQuad := world.NewGraphQuadWithKeys(key, TypePred.String(), BuildTypeObjectKey(typeID), "")
	quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, TypePred.String(), "", ""), typeGraphLookupLimit)
	if err != nil {
		return err
	}

	// Remove stale type edges and remember whether the requested edge exists.
	exists := false
	for _, q := range quads {
		if q.GetObj() == nextQuad.GetObj() {
			exists = true
			continue
		}
		if err := ws.DeleteGraphQuad(ctx, q); err != nil {
			return err
		}
	}

	// Preserve the matching type edge without writing it again.
	if exists {
		return nil
	}
	return ws.SetGraphQuad(ctx, nextQuad)
}

// EnsureTypeExists creates the object representing the type ID if it doesn't exist.
//
// The world state answers HasObject from transaction-local knowledge when it
// can, so repeated calls with the same type within one transaction avoid
// re-reading the type object. Creating the object only when HasObject reports it
// absent is safe under the single-writer transaction discipline; concurrent
// creation is not a concern within one transaction.
func EnsureTypeExists(ctx context.Context, ws world.WorldState, typeID string) error {
	// Check whether the World already contains the type object.
	objKey := BuildTypeObjectKey(typeID)
	exists, err := ws.HasObject(ctx, objKey)
	if err != nil {
		return err
	}

	// Create the missing type object and release its acquired state.
	if !exists {
		obj, err := ws.CreateObject(ctx, objKey, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
	}
	return nil
}

// IterateObjectsWithType iterates over object keys with the given type ID.
func IterateObjectsWithType(
	rctx context.Context,
	ws world.WorldState,
	typeID string,
	cb func(objKey string) (bool, error),
) error {
	// Require a type identifier before visiting matching objects.
	if typeID == "" {
		return ErrTypeIDEmpty
	}

	// Leave the World untouched when no object visitor is supplied.
	if cb == nil {
		return nil
	}

	// Visit type matches through the World object listing API when available.
	if lister, ok := ws.(ObjectTypeLister); ok {
		// Fetch the object keys for the requested type.
		objKeys, err := lister.ListObjectsWithType(rctx, typeID)
		if err != nil {
			return err
		}

		// Visit each listed object until the callback stops the traversal.
		for _, objKey := range objKeys {
			ctnu, err := cb(objKey)
			if err != nil || !ctnu {
				return err
			}
		}
		return nil
	}

	// Resolve incoming type edges to their object keys.
	objKeys, err := world.CollectGraphPathStepWithKeys(
		rctx,
		ws,
		[]string{BuildTypeObjectKey(typeID)},
		world.GraphPathDirectionIn,
		TypePred.String(),
		typeGraphLookupLimit,
	)
	if err != nil {
		return err
	}

	// Visit each graph match until the callback stops the traversal.
	for _, objKey := range objKeys {
		ctnu, err := cb(objKey)
		if err != nil || !ctnu {
			return err
		}
	}
	return nil
}

// ListObjectsWithType returns the list of object keys with the given type id.
func ListObjectsWithType(ctx context.Context, ws world.WorldState, typeID string) ([]string, error) {
	// Require a type identifier before collecting matching object keys.
	if typeID == "" {
		return nil, ErrTypeIDEmpty
	}

	// Delegate object collection to the World listing API when available.
	if lister, ok := ws.(ObjectTypeLister); ok {
		return lister.ListObjectsWithType(ctx, typeID)
	}

	// Collect every object key visited by the type traversal.
	var objKeys []string
	err := IterateObjectsWithType(ctx, ws, typeID, func(objKey string) (bool, error) {
		objKeys = append(objKeys, objKey)
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return objKeys, nil
}

// ListCollectObjectsWithType returns the list of object keys with the given type id.
// Unmarshals the bodies of the matched objects.
//
// ctor must return an object of type T.
// Returns two slices of length len(objKeys). If any objects are not found,
// their entries are nil and ErrNotFound is returned after all states release.
func ListCollectObjectsWithType[T block.Block](ctx context.Context, ws world.WorldState, typeID string, ctor func() block.Block) ([]T, []string, error) {
	// Resolve the object keys whose bodies belong to the requested type.
	objKeys, err := ListObjectsWithType(ctx, ws, typeID)
	if err != nil {
		return nil, nil, err
	}

	// Decode the matching bodies and release every acquired object state.
	objs, states, err := world.CollectObjectBodies[T](ctx, ws, objKeys, ctor)
	for _, state := range states {
		world.ReleaseObjectState(state)
	}
	return objs, objKeys, err
}
