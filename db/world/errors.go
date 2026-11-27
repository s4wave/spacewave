package world

import (
	"context"
	"errors"
	"fmt"

	"github.com/s4wave/spacewave/db/coord"

	"github.com/aperturerobotics/cayley/graph"
)

var (
	// ErrHistoryUnavailable is returned if the world storage implementation
	// does not implement looking up the world state at a given seqno.
	ErrHistoryUnavailable = errors.New("world history not available")
	// ErrWorldStorageUnavailable is returned if world storage access is unavailable.
	ErrWorldStorageUnavailable = errors.New("world storage not available")
	// ErrStateNotFound is returned if the state lookup returned not found.
	ErrStateNotFound = errors.New("world state not found")
	// ErrObjectExists returns if the object exists already.
	ErrObjectExists = errors.New("object already exists")
	// ErrObjectNotFound is returned if an object was not found.
	// Note: this is only returned in error conditions.
	// Most lookup functions return value, ok, error.
	ErrObjectNotFound = errors.New("object not found")
	// ErrQuadNotFound is returned if a graph quad was not found.
	// Note: this is only returned in error conditions.
	// Most lookup functions return value, ok, error.
	ErrQuadNotFound = graph.ErrQuadNotExist
	// ErrEmptyObjectKey returns if the object key was empty.
	ErrEmptyObjectKey = errors.New("object key cannot be empty")
	// ErrUnhandledOp is returned if the operation type was unhandled.
	ErrUnhandledOp = errors.New("operation type was not handled")
	// ErrEmptyOp is returned if the operation type ID or operation object are empty.
	ErrEmptyOp = errors.New("operation type id and body cannot be empty")
	// ErrEmptyChange is returned if the changelog entry was empty.
	ErrEmptyChange = errors.New("changelog entry cannot be empty")
	// ErrEmptyEngineID is returned if the engine id was empty.
	ErrEmptyEngineID = errors.New("engine id cannot be empty")
	// ErrSuperseded is returned for a candidate another candidate replaced as
	// the head after its base. The writer rebases and runs its change again.
	ErrSuperseded = errors.New("world candidate superseded")
	// ErrEngineRetired is returned by a World engine its owner retired, and by
	// the transactions it ended open, once a successor serves the World: the
	// caller runs its change again on the successor. It is a supersession.
	ErrEngineRetired = fmt.Errorf("world engine retired: %w", ErrSuperseded)
	// ErrDurabilityUnknown is returned for a commit whose candidate may have
	// become durable when the wait for its decision ended, such as at an
	// unmount. It never means the commit failed: the next open reconciles
	// the head, so the writer reads the World before it writes again.
	ErrDurabilityUnknown = errors.New("world commit durability unknown")
	// ErrOutcomeHidden is returned by WaitSettled for an operation a
	// checkpoint folded in without recording whether it applied. The World
	// holds its effect or not, but no record names which.
	ErrOutcomeHidden = errors.New("world checkpoint hides the operation outcome")
	// ErrSettleUnsupported is returned by WaitSettled on an engine that
	// cannot settle an unknown write.
	ErrSettleUnsupported = errors.New("world engine cannot settle an unknown write")
	// ErrUnexpectedChangeType is returned when a change with an unexpected
	// ChangeType is encountered. Typically wrapped with name of type.
	ErrUnexpectedChangeType = errors.New("change type was unexpected")
	// ErrUnexpectedRev is returned if the revision field was unexpected.
	ErrUnexpectedRev = errors.New("object rev unexpected")
	// ErrGraphPathResultLimit is returned if a graph path query has no result limit.
	ErrGraphPathResultLimit = errors.New("graph path result limit must be non-zero")
	// ErrGraphPathStepLimit is returned if a graph path step has no per-key limit.
	ErrGraphPathStepLimit = errors.New("graph path step limit must be non-zero")
	// ErrGraphPathPredicate is returned if a graph path step has no predicate.
	ErrGraphPathPredicate = errors.New("graph path step predicate cannot be empty")
	// ErrGraphPathDirection is returned if a graph path step has no valid direction.
	ErrGraphPathDirection = errors.New("graph path step direction is invalid")
	// ErrGraphEdgeBucketLimit is returned if a grouped edge query has no per-origin limit.
	ErrGraphEdgeBucketLimit = errors.New("graph edge bucket limit must be non-zero")
	// ErrGraphEdgeBucketDirection is returned if a grouped edge query direction is invalid.
	ErrGraphEdgeBucketDirection = errors.New("graph edge bucket direction is invalid")
	// ErrGraphEdgeBucketAfter is returned if a grouped edge query resumes after
	// an endpoint without a predicate to page.
	ErrGraphEdgeBucketAfter = errors.New("graph edge bucket after requires a predicate")
	// ErrGraphEdgeBucketAfterGone is returned if a grouped edge query resumes
	// after an endpoint that no graph quad references any longer, so its
	// place in the order is lost. The reader restarts from the first page.
	ErrGraphEdgeBucketAfterGone = errors.New("graph edge bucket after endpoint is gone")

	// ErrNilQuad is returned if the quad is nil and cannot be.
	ErrNilQuad = errors.New("quad cannot be nil")
	// ErrEmptyQuadSubject is returned if the subject field was empty.
	ErrEmptyQuadSubject = errors.New("quad subject cannot be empty")
	// ErrEmptyQuadPred is returned if the predicate field was empty.
	ErrEmptyQuadPred = errors.New("quad predicate cannot be empty")
	// ErrEmptyQuadObject is returned if the object field was empty.
	ErrEmptyQuadObject = errors.New("quad object cannot be empty")
	// ErrQuadSubjectNotIRI indicates a quad subject must be an IRI.
	ErrQuadSubjectNotIRI = errors.New("quad subject must be an iri")
	// ErrQuadObjectNotIRI indicates a quad object must be an IRI.
	ErrQuadObjectNotIRI = errors.New("quad object must be an iri")
	// ErrNotIRI is returned if the format <object-key> is not used for the graph key.
	ErrNotIRI = errors.New("quad value must be valid object IRIs")
)

// IsSuperseded reports whether err says another writer moved the head after
// the write's base, so the write may run again from a new transaction. Every
// retry site matches through it, so a change of the sentinel reaches them all.
// coord.ErrStaleGeneration counts too: a coordinator reports a write lease
// that lost its generation with it.
//
// An error that wraps ErrDurabilityUnknown is never superseded, whatever else
// it wraps: the write may be durable, so no retrier may run it again.
func IsSuperseded(err error) bool {
	if errors.Is(err, ErrDurabilityUnknown) {
		return false
	}
	return errors.Is(err, ErrSuperseded) || errors.Is(err, coord.ErrStaleGeneration)
}

// DurabilityUnknownError is an ErrDurabilityUnknown for one signed operation.
// OpHash names the operation, so the writer can wait for a successor engine
// to settle whether it applied before it writes again.
type DurabilityUnknownError struct {
	// OpHash is the hash of the signed operation. The caller must not modify it.
	OpHash []byte
	// Err is the cause.
	Err error
}

// Error returns the cause behind the ErrDurabilityUnknown text.
func (e *DurabilityUnknownError) Error() string {
	if e.Err == nil {
		return ErrDurabilityUnknown.Error()
	}
	return ErrDurabilityUnknown.Error() + ": " + e.Err.Error()
}

// Is reports that the error is an ErrDurabilityUnknown.
func (e *DurabilityUnknownError) Is(target error) bool {
	return target == ErrDurabilityUnknown
}

// Unwrap returns the cause.
func (e *DurabilityUnknownError) Unwrap() error {
	return e.Err
}

// SettleWaiter settles a write whose commit ended with DurabilityUnknownError.
type SettleWaiter interface {
	// WaitSettled waits until an engine whose storage shows whether the
	// operation of opHash was stored settles it, and returns that engine's
	// head and whether the operation applied. That engine either replayed the
	// operation, or reloaded the World's storage after the engine that
	// returned the unknown ended its writes and found it absent. The writer
	// holds its own writes until it returns: one that applied is in head, and
	// one that did not was never stored and no engine can store it later.
	//
	// With the reload fence on, the old engine commits nothing after the
	// unknown, so head is the state at the unknown, with the operation when
	// it applied, plus what other devices wrote. With the fence off
	// (sobject.ReloadFenceOff) the old engine keeps committing until the
	// reload ends its run, and head may also hold this device's writes that
	// followed the unknown.
	//
	// It returns the answer once: a later call for opHash returns
	// ErrNoSettle, as does a hash no write ended unknown with. It returns
	// ErrOutcomeHidden when a checkpoint folded the operation in without its
	// outcome. It works after the engine that returned the unknown retired,
	// since a successor settles the write, and waits until ctx ends while no
	// engine can show the outcome.
	WaitSettled(ctx context.Context, opHash []byte) (head Head, applied bool, err error)
}
