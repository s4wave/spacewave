package world

import (
	"context"
	"strings"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/cayleygraph/cayley/graph"
	"github.com/cayleygraph/quad"
)

// AccessWorldStateFunc is a function to access world state.
// Ref can be nil to indicate accessing context-specific default.
type AccessWorldStateFunc = func(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error

// WorldState is the state read/write operations interface.
type WorldState interface {
	// GetReadOnly returns if the state is read-only.
	GetReadOnly() bool
	// GetSeqno returns the current seqno of the world state.
	// This is also the sequence number of the most recent change.
	// Initializes at 0 for initial world state.
	GetSeqno() (uint64, error)

	// AccessWorldState builds a bucket lookup cursor with an optional ref.
	// If the ref is empty, returns empty cursor in the same bucket + volume as the world.
	// The lookup cursor will be released after cb returns.
	AccessWorldState(
		ctx context.Context,
		ref *bucket.ObjectRef,
		cb func(*bucket_lookup.Cursor) error,
	) error
	// ApplyWorldOp applies a batch operation at the world level.
	// The handling of the operation is operation-type specific.
	// Returns the seqno following the operation execution.
	// If nil is returned for the error, implies success.
	ApplyWorldOp(operationTypeID string, op Operation, opSender peer.ID) (uint64, error)
	// WorldStateObject contains the object APIs
	WorldStateObject
	// WorldStateGraph contains the graph APIs
	WorldStateGraph
}

// NewAccessWorldStateFunc constructs an AccessWorldStateFunc from a existing cursor
func NewAccessWorldStateFunc(cursor *bucket_lookup.Cursor) AccessWorldStateFunc {
	return func(
		ctx context.Context,
		ref *bucket.ObjectRef,
		cb func(*bucket_lookup.Cursor) error,
	) error {
		ncs := cursor.Clone()
		defer ncs.Release()
		return cb(ncs)
	}
}

// WorldStateObject contains the object APIs on WorldState.
type WorldStateObject interface {
	// CreateObject creates a object with a key and initial root ref.
	// Returns ErrObjectExists if the object already exists.
	// Appends a OBJECT_SET change to the changelog.
	CreateObject(key string, rootRef *bucket.ObjectRef) (ObjectState, error)
	// GetObject looks up an object by key.
	// Returns nil, false if not found.
	GetObject(key string) (ObjectState, bool, error)
	// DeleteObject deletes an object and associated graph quads by ID.
	// Calls DeleteGraphObject internally.
	// Returns false, nil if not found.
	DeleteObject(key string) (bool, error)
}

// WorldStateGraph contains the graph APIs on WorldState.
type WorldStateGraph interface {
	// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
	// All accesses of the handle should complete before returning cb.
	// Try to make access (queries) as short as possible.
	// Write operations will fail if the store is read-only.
	AccessCayleyGraph(write bool, cb func(h CayleyHandle) error) error
	// LookupGraphQuads searches for graph quads in the store.
	// If the filter fields are empty, matches any for that field.
	// If not found, returns nil, nil
	// If limit is set, stops after finding that number of matching quads.
	LookupGraphQuads(filter GraphQuad, limit uint32) ([]GraphQuad, error)
	// SetGraphQuad sets a quad in the graph store.
	// Subject: must be an existing object IRI: <object-id>
	// Predicate: a predicate string, e.x. IRI: <ref>
	// Object: an existing object IRI: <object-id>
	// If already exists, returns nil.
	SetGraphQuad(q GraphQuad) error
	// DeleteGraphQuad deletes a quad from the graph store.
	// Note: if quad did not exist, returns nil.
	DeleteGraphQuad(q GraphQuad) error
	// DeleteGraphObject deletes all quads with Subject or Object set to value.
	// May also remove objects with <predicate> or <value> set to the value.
	// Returns number of removed quads and any error.
	DeleteGraphObject(value string) error
}

// CayleyHandle is a cayley graph handle.
// Note: QuadWriter is not included, writes must be done with WorldStateGraph.
type CayleyHandle interface {
	graph.QuadStore
	// graph.QuadWriter
}

// KeyToGraphValue is the string representation of the key for a graph IRI.
func KeyToGraphValue(key string) quad.Value {
	if key == "" {
		return nil
	}
	return quad.IRI(key)
}

// QuadValueToKey attempts to convert a graph value to a quad.IRI and then string.
// use with GraphQuadStringToCayleyValue
func QuadValueToKey(gv quad.Value) (string, error) {
	if gv == nil {
		return "", nil
	}
	iri, ok := gv.(quad.IRI)
	if ok {
		return string(iri), nil
	}
	return GraphValueToKey(gv.String())
}

// GraphValueToKey attempts to convert a graph value to a quad.IRI and then string.
// use with GraphQuadStringToCayleyValue
func GraphValueToKey(gv string) (string, error) {
	iri, err := GraphEnsureIsIRI(gv)
	if err != nil {
		return "", err
	}
	return string(iri), nil
}

// GraphEnsureIsIRI confirms that a string is an IRI.
func GraphEnsureIsIRI(val string) (quad.IRI, error) {
	if !strings.HasPrefix(val, "<") || !strings.HasSuffix(val, ">") {
		return quad.IRI(""), ErrNotIRI
	}
	return quad.IRI(val[1 : len(val)-1]), nil
}

// MustGetObject looks up an object in a world state or returns ErrObjectNotFound.
func MustGetObject(w WorldStateObject, key string) (ObjectState, error) {
	obj, found, err := w.GetObject(key)
	if err == nil && !found {
		err = ErrObjectNotFound
	}
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// AccessObject is a utility for AccessWorldState to create a ObjectRef.
// Ref can be nil to indicate creating a new object.
// The block transaction is written upon completion and updated ObjectRef returned.
//
// Returns the updated object ref and any error.
func AccessObject(
	ctx context.Context,
	access AccessWorldStateFunc,
	ref *bucket.ObjectRef,
	cb func(bcs *block.Cursor) error,
) (*bucket.ObjectRef, error) {
	outRef := ref.Clone()
	if outRef == nil {
		outRef = &bucket.ObjectRef{}
	}
	err := access(ctx, ref, func(bls *bucket_lookup.Cursor) error {
		btx, bcs := bls.BuildTransaction(nil)
		berr := cb(bcs)
		if berr != nil {
			return berr
		}
		outRef.RootRef, _, berr = btx.Write(true)
		return berr
	})
	return outRef, err
}
