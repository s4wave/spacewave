package block_gc

import (
	"strings"

	"github.com/aperturerobotics/hydra/block"
)

// IRI prefix constants for constructing graph node identifiers.
const (
	prefixBlock  = "block:"
	prefixObject = "object:"
	prefixBucket = "bucket:"
)

// Well-known node IRIs (permanent roots).
const (
	NodeGCRoot       = "gcroot"
	NodeUnreferenced = "unreferenced"
)

// PredGCRef is the single predicate for all GC reference edges.
const PredGCRef = "gc/ref"

// BlockIRI returns the IRI for a block reference: "block:{b58hash}".
func BlockIRI(ref *block.BlockRef) string {
	if ref == nil || ref.GetEmpty() {
		return ""
	}
	return prefixBlock + ref.MarshalString()
}

// ParseBlockIRI parses a "block:{b58}" IRI back to a BlockRef.
// Returns nil, false if not a valid block IRI.
func ParseBlockIRI(iri string) (*block.BlockRef, bool) {
	rest, ok := strings.CutPrefix(iri, prefixBlock)
	if !ok || rest == "" {
		return nil, false
	}
	ref, err := block.UnmarshalBlockRefB58(rest)
	if err != nil {
		return nil, false
	}
	return ref, true
}

// ObjectIRI returns "object:{key}" for a world object.
func ObjectIRI(key string) string {
	return prefixObject + key
}

// parseObjectIRI parses an "object:{key}" IRI back to the object key.
// Returns the key and true if valid.
func parseObjectIRI(iri string) (string, bool) {
	return strings.CutPrefix(iri, prefixObject)
}

// BucketIRI returns the IRI for a bucket node: "bucket:{bucketID}".
func BucketIRI(bucketID string) string {
	return prefixBucket + bucketID
}

// ParseBucketIRI parses a "bucket:{id}" IRI back to a bucket ID.
// Returns the bucket ID and true if valid.
func ParseBucketIRI(iri string) (string, bool) {
	rest, ok := strings.CutPrefix(iri, prefixBucket)
	if !ok || rest == "" {
		return "", false
	}
	return rest, true
}

// IsPermanentRoot returns true if the IRI is a well-known permanent root.
func IsPermanentRoot(iri string) bool {
	return iri == NodeGCRoot || iri == NodeUnreferenced
}
