package world_vlogger

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	"github.com/sirupsen/logrus"
)

// WorldState implements a WorldState wrapped with verbose logging.
type WorldState struct {
	// WorldState is the underlying WorldState object.
	world.WorldState

	// le is the logger
	le *logrus.Entry
}

// NewWorldState constructs a new world state vlogger.
func NewWorldState(le *logrus.Entry, worldState world.WorldState) *WorldState {
	return &WorldState{
		WorldState: worldState,
		le:         le,
	}
}

// ApplyWorldOp applies a batch operation at the world level.
// The handling of the operation is operation-type specific.
// Returns the seqno following the operation execution.
// If nil is returned for the error, implies success.
// If sysErr is set, the error is treated as a transient system error.
// Must support recursive calls to ApplyWorldOp / ApplyObjectOp.
// Returns seqno, sysErr, err
func (w *WorldState) ApplyWorldOp(
	op world.Operation,
	opSender peer.ID,
) (seqno uint64, sysErr bool, err error) {
	if op == nil {
		return 0, false, world.ErrEmptyOp
	}
	defer func() {
		w.le.Debugf(
			"ApplyWorldOp(%s, %s) => seqno(%v) sysErr(%v) err(%v)",
			op.GetOperationTypeId(),
			opSender.Pretty(),
			seqno, sysErr, err,
		)
	}()
	return w.WorldState.ApplyWorldOp(op, opSender)
}

// CreateObject creates a object with a key and initial root ref.
// Returns ErrObjectExists if the object already exists.
// Appends a OBJECT_SET change to the changelog.
func (w *WorldState) CreateObject(key string, rootRef *bucket.ObjectRef) (_ world.ObjectState, err error) {
	defer func() {
		w.le.Debugf(
			"CreateObject(%s, %s) => err(%v)",
			key, rootRef.MarshalString(),
			err,
		)
	}()
	return w.WorldState.CreateObject(key, rootRef)
}

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (w *WorldState) GetObject(key string) (_ world.ObjectState, found bool, err error) {
	defer func() {
		w.le.Debugf(
			"GetObject(%s) => found(%v) err(%v)",
			key, found, err,
		)
	}()
	return w.WorldState.GetObject(key)
}

// DeleteObject deletes an object and associated graph quads by ID.
// Calls DeleteGraphObject internally.
// Returns false, nil if not found.
func (w *WorldState) DeleteObject(key string) (found bool, err error) {
	defer func() {
		w.le.Debugf(
			"DeleteObject(%s) => found(%v) err(%v)",
			key, found, err,
		)
	}()
	return w.WorldState.DeleteObject(key)
}

// LookupGraphQuads searches for graph quads in the store.
// If the filter fields are empty, matches any for that field.
// If not found, returns nil, nil
// If limit is set, stops after finding that number of matching quads.
func (w *WorldState) LookupGraphQuads(filter world.GraphQuad, limit uint32) (qs []world.GraphQuad, err error) {
	defer func() {
		cq, _ := world.GraphQuadToCayleyQuad(filter, false)
		w.le.Debugf(
			"LookupGraphQuads(%s, %d) => found(%d) err(%v)",
			cq.String(), limit, len(qs), err,
		)
	}()
	return w.WorldState.LookupGraphQuads(filter, limit)
}

// SetGraphQuad sets a quad in the graph store.
// Subject: must be an existing object IRI: <object-key>
// Predicate: a predicate string, e.x. IRI: <ref>
// Object: an existing object IRI: <object-key>
// If already exists, returns nil.
func (w *WorldState) SetGraphQuad(q world.GraphQuad) (err error) {
	defer func() {
		cq, _ := world.GraphQuadToCayleyQuad(q, false)
		w.le.Debugf(
			"SetGraphQuad(%s) => err(%v)",
			cq.String(), err,
		)
	}()
	return w.WorldState.SetGraphQuad(q)
}

// DeleteGraphQuad deletes a quad from the graph store.
// Note: if quad did not exist, returns nil.
func (w *WorldState) DeleteGraphQuad(q world.GraphQuad) (err error) {
	defer func() {
		cq, _ := world.GraphQuadToCayleyQuad(q, false)
		w.le.Debugf(
			"DeleteGraphQuad(%s) => err(%v)",
			cq.String(), err,
		)
	}()
	return w.WorldState.DeleteGraphQuad(q)
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
// Note: value should be the object key, NOT the object key <iri> format.
func (w *WorldState) DeleteGraphObject(value string) (err error) {
	defer func() {
		w.le.Debugf(
			"DeleteGraphObject(%s) => err(%v)",
			value, err,
		)
	}()
	return w.WorldState.DeleteGraphObject(value)
}

// _ is a type assertion
var _ world.WorldState = ((*WorldState)(nil))
