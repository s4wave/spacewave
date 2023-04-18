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

// WorldStorage allows accessing the world storage via bucket lookup.
type WorldStorage interface {
	// BuildStorageCursor builds a cursor to the world storage with an empty ref.
	// The cursor should be released independently of the WorldState.
	// Be sure to call Release on the cursor when done.
	BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error)

	// AccessWorldState builds a bucket lookup cursor with an optional ref.
	// If the ref is empty, returns a cursor pointing to the root world state.
	// The lookup cursor will be released after cb returns.
	AccessWorldState(
		ctx context.Context,
		ref *bucket.ObjectRef,
		cb func(*bucket_lookup.Cursor) error,
	) error
}

// WorldWaitSeqno allows readers to wait for a minimum state sequence number.
type WorldWaitSeqno interface {
	// GetSeqno returns the current seqno of the world state.
	// This is also the sequence number of the most recent change.
	// Initializes at 0 for initial world state.
	GetSeqno() (uint64, error)

	// WaitSeqno waits for the seqno of the world state to be >= value.
	// Returns the seqno when the condition is reached.
	// If value == 0, this might return immediately unconditionally.
	WaitSeqno(ctx context.Context, value uint64) (uint64, error)
}

// WorldState is the state read/write operations interface.
type WorldState interface {
	// GetReadOnly returns if the state is read-only.
	GetReadOnly() bool

	// WorldStorage accesses the world storage.
	WorldStorage
	// WorldStateObject contains the object APIs
	WorldStateObject
	// WorldStateGraph contains the graph APIs
	WorldStateGraph
	// WorldStateOp contains the operation APIs.
	WorldStateOp

	// WorldWaitSeqno waits for the world state to change.
	WorldWaitSeqno
}

// ForkableWorldState adds a Fork function to the WorldState, which returns an
// independent WorldState with a new underlying tx.
type ForkableWorldState interface {
	WorldState

	// Fork forks the current state into a new state.
	Fork(ctx context.Context) (WorldState, error)
}

// WorldStateOp contains the operation APIs on WorldState.
type WorldStateOp interface {
	// ApplyWorldOp applies a batch operation at the world level.
	// The handling of the operation is operation-type specific.
	// Returns the seqno following the operation execution.
	// If nil is returned for the error, implies success.
	// If sysErr is set, the error is treated as a transient system error.
	// Must support recursive calls to ApplyWorldOp / ApplyObjectOp.
	// Returns seqno, sysErr, err
	ApplyWorldOp(
		op Operation,
		opSender peer.ID,
	) (seqno uint64, sysErr bool, err error)
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
	// Subject: must be an existing object IRI: <object-key>
	// Predicate: a predicate string, e.x. IRI: <ref>
	// Object: an existing object IRI: <object-key>
	// If already exists, returns nil.
	SetGraphQuad(q GraphQuad) error
	// DeleteGraphQuad deletes a quad from the graph store.
	// Note: if quad did not exist, returns nil.
	DeleteGraphQuad(q GraphQuad) error
	// DeleteGraphObject deletes all quads with Subject or Object set to value.
	// Note: value should be the object key, NOT the object key <iri> format.
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

// GraphValueToString calls String() on the GraphValue if it's not nil.
func GraphValueToString(gv quad.Value) string {
	if gv == nil {
		return ""
	}
	return gv.String()
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

// AccessObjectCb is a callback to access a block cursor.
type AccessObjectCb func(bcs *block.Cursor) error

// AccessObject is a utility for AccessWorldState to access or create an ObjectRef.
// Ref can be nil to indicate creating a new object.
// The block transaction is written upon completion and updated ObjectRef returned.
//
// Returns the updated object ref and any error.
func AccessObject(
	ctx context.Context,
	access AccessWorldStateFunc,
	ref *bucket.ObjectRef,
	cb AccessObjectCb,
) (*bucket.ObjectRef, error) {
	var outRef *bucket.ObjectRef
	err := access(ctx, ref, func(bls *bucket_lookup.Cursor) error {
		btx, bcs := bls.BuildTransaction(nil)
		if ref.GetEmpty() {
			// bcs.SetBlock(nil, false)
			bcs.SetRefAtCursor(nil, true)
		}
		berr := cb(bcs)
		if berr != nil {
			return berr
		}
		outRef = bls.GetRef().Clone()
		outRef.RootRef, _, berr = btx.Write(true)
		return berr
	})
	return outRef, err
}

// CreateWorldObject is a utility for WorldState to create a Object.
//
// Returns the updated object ref and any error.
func CreateWorldObject(
	ctx context.Context,
	ws WorldState,
	objKey string,
	cb AccessObjectCb,
) (ObjectState, *bucket.ObjectRef, error) {
	_, exists, err := ws.GetObject(objKey)
	if err == nil && exists {
		err = ErrObjectExists
	}
	if err != nil {
		return nil, nil, err
	}

	objRef, err := AccessObject(ctx, ws.AccessWorldState, nil, cb)
	if err != nil {
		return nil, nil, err
	}
	objs, err := ws.CreateObject(objKey, objRef)
	return objs, objRef, err
}

// AccessWorldObject attempts to look up an object in the world state.
// If the object did not exist, bcs will be empty, the object will be created.
// If updateWorld=true, and the result is different, will SetRootRef with change.
// Note: if updateWorld=true but ws is read-only, sets updateWorld=false.
// Returns the modified object ref, if it was dirty, and any error.
func AccessWorldObject(
	ctx context.Context,
	ws WorldState,
	objKey string,
	updateWorld bool,
	cb AccessObjectCb,
) (*bucket.ObjectRef, bool, error) {
	if ws.GetReadOnly() {
		updateWorld = false
	}

	obj, existed, err := ws.GetObject(objKey)
	if err != nil {
		return nil, false, err
	}

	// create object from scratch if it didn't exist.
	if !existed {
		initRef, err := AccessObject(ctx, ws.AccessWorldState, nil, cb)
		if err == nil && updateWorld {
			_, err = ws.CreateObject(objKey, initRef)
		}
		return initRef, true, err
	}

	return AccessObjectState(ctx, obj, updateWorld, cb)
}

// AccessObjectState accesses and updates a world object handle if updateWorld is set.
// If updateWorld=true, and the result is different, will SetRootRef with change.
// Note: if updateWorld=true but ws is read-only, sets updateWorld=false.
// Returns the modified object ref, if it was dirty, and any error.
func AccessObjectState(
	ctx context.Context,
	obj ObjectState,
	updateWorld bool,
	cb AccessObjectCb,
) (*bucket.ObjectRef, bool, error) {
	if obj == nil {
		return nil, false, ErrObjectNotFound
	}
	initRef, _, err := obj.GetRootRef()
	if err != nil {
		return nil, false, err
	}
	outRef, err := AccessObject(ctx, obj.AccessWorldState, initRef, cb)
	if err != nil {
		return nil, false, err
	}
	var dirty bool
	if initRef.GetBucketId() != "" && initRef.GetBucketId() != outRef.GetBucketId() {
		dirty = true
	}
	if !outRef.GetRootRef().EqualsRef(initRef.GetRootRef()) {
		dirty = true
	}
	if updateWorld && dirty {
		_, err = obj.SetRootRef(outRef)
	}
	return outRef, dirty, err
}
