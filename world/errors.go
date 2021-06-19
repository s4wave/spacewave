package world

import "errors"

var (
	// ErrHistoryUnavailable is returned if the world storage implementation
	// does not implement looking up the world state at a given seqno.
	ErrHistoryUnavailable = errors.New("world history not available")
	// ErrStateNotFound is returned if the state lookup returned not found.
	ErrStateNotFound = errors.New("world state not found")
	// ErrObjectExists returns if the object exists already.
	ErrObjectExists = errors.New("object already exists")
	// ErrObjectNotFound is returned if an object was not found.
	// Note: this is only returned in error conditions.
	// Most lookup functions return value, ok, error.
	ErrObjectNotFound = errors.New("object not found")
	// ErrEmptyObjectKey returns if the object key was empty.
	ErrEmptyObjectKey = errors.New("object key cannot be empty")
	// ErrUnhandledOp is returned if the operation type was unhandled.
	ErrUnhandledOp = errors.New("operation type was not handled")
	// ErrEmptyOp is returned if the operation type ID or operation object are empty.
	ErrEmptyOp = errors.New("operation type id and body cannot be empty")

	// ErrNilQuad is returned if the quad is nil and cannot be.
	ErrNilQuad = errors.New("quad cannot be nil")
	// ErrEmptyQuadSubject is returned if the subject field was empty.
	ErrEmptyQuadSubject = errors.New("quad subject cannot be empty")
	// ErrEmptyQuadPred is returned if the predicate field was empty.
	ErrEmptyQuadPred = errors.New("quad predicate cannot be empty")
	// ErrEmptyQuadObject is returned if the object field was empty.
	ErrEmptyQuadObject = errors.New("quad predicate cannot be empty")
	// ErrQuadSubjectNotIRI indicates a quad subject must be an IRI.
	ErrQuadSubjectNotIRI = errors.New("quad subject must be an iri")
	// ErrQuadObjectNotIRI indicates a quad object must be an IRI.
	ErrQuadObjectNotIRI = errors.New("quad object must be an iri")
	// ErrNotIRI is returned if the format <object-id> is not used for the graph key.
	ErrNotIRI = errors.New("quad value must be valid object IRIs")
)
