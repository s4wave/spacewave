package world

import "github.com/aperturerobotics/hydra/bucket"

// WorldState is the state read/write operations interface.
type WorldState interface {
	// GetReadOnly returns if the state is read-only.
	GetReadOnly() bool

	// WorldStateObject contains the object APIs
	WorldStateObject
	// WorldStateGraph contains the graph APIs
	WorldStateGraph
}

// WorldStateObject contains the object APIs on WorldState.
type WorldStateObject interface {
	// CreateObject creates an empty object with a key.
	// Returns ErrObjectExists if the object already exists.
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

// MustGetObject looks up an object in a world state or returns ErrObjectNotFound.
func MustGetObject(w WorldState, key string) (ObjectState, error) {
	obj, found, err := w.GetObject(key)
	if err == nil && !found {
		err = ErrObjectNotFound
	}
	if err != nil {
		return nil, err
	}
	return obj, nil
}
