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
	if batcher, ok := ws.(ObjectMetadataBatcher); ok {
		metadata, err := batcher.GetObjectMetadataBatch(ctx, []string{key})
		if err != nil || len(metadata) == 0 {
			return "", err
		}
		return metadata[0].TypeID, nil
	}

	var typeKey string
	quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, TypePred.String(), "", ""), typeGraphLookupLimit)
	if err != nil {
		return "", err
	}
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
	if key == "" || typeID == "" {
		return world.ErrEmptyObjectKey
	}

	// check that the object representing the type exists and create it if not
	if _, err := EnsureTypeExists(ctx, ws, typeID); err != nil {
		return err
	}

	nextQuad := world.NewGraphQuadWithKeys(key, TypePred.String(), BuildTypeObjectKey(typeID), "")
	quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, TypePred.String(), "", ""), typeGraphLookupLimit)
	if err != nil {
		return err
	}
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
	if exists {
		return nil
	}
	return ws.SetGraphQuad(ctx, nextQuad)
}

// EnsureTypeExists creates the object representing the type ID if it doesn't exist.
func EnsureTypeExists(ctx context.Context, ws world.WorldState, typeID string) (created bool, err error) {
	objKey := BuildTypeObjectKey(typeID)
	_, existed, err := ws.GetObject(ctx, objKey)
	if err != nil {
		return false, err
	}
	if existed {
		return true, nil
	}
	if _, err = ws.CreateObject(ctx, objKey, nil); err != nil {
		return false, err
	}
	return true, nil
}

// IterateObjectsWithType iterates over object keys with the given type ID.
func IterateObjectsWithType(
	rctx context.Context,
	ws world.WorldState,
	typeID string,
	cb func(objKey string) (bool, error),
) error {
	if typeID == "" {
		return ErrTypeIDEmpty
	}
	if cb == nil {
		return nil
	}

	if lister, ok := ws.(ObjectTypeLister); ok {
		objKeys, err := lister.ListObjectsWithType(rctx, typeID)
		if err != nil {
			return err
		}
		for _, objKey := range objKeys {
			ctnu, err := cb(objKey)
			if err != nil || !ctnu {
				return err
			}
		}
		return nil
	}

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
	if typeID == "" {
		return nil, ErrTypeIDEmpty
	}
	if lister, ok := ws.(ObjectTypeLister); ok {
		return lister.ListObjectsWithType(ctx, typeID)
	}

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
// ctor must return an object of type T
// returns two slices of length objKeys
// if any objects are not found, returns nil for that object state / value and objs, objsStates, ErrNotFound
// returns nil, nil, err for any other error
func ListCollectObjectsWithType[T block.Block](ctx context.Context, ws world.WorldState, typeID string, ctor func() block.Block) ([]T, []string, error) {
	objKeys, err := ListObjectsWithType(ctx, ws, typeID)
	if err != nil {
		return nil, nil, err
	}

	objs, _, err := world.CollectObjectBodies[T](ctx, ws, objKeys, ctor)
	return objs, objKeys, err
}
