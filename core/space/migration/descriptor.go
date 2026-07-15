package space_migration

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// ReferenceKind identifies the owner and rewrite semantics for one embedded identity.
type ReferenceKind int32

const (
	// ReferenceObjectKey identifies a Space-local object key.
	ReferenceObjectKey ReferenceKind = 1
	// ReferenceNestedSharedObject identifies a nested SharedObject ID.
	ReferenceNestedSharedObject ReferenceKind = 2
	// ReferenceCanvasNode identifies a Canvas node ID.
	ReferenceCanvasNode ReferenceKind = 3
	// ReferenceGraphIRI identifies a graph IRI.
	ReferenceGraphIRI ReferenceKind = 4
	// ReferenceExternal identifies an intentionally external reference.
	ReferenceExternal ReferenceKind = 5
	// ReferenceBlockStore identifies a block-store identity.
	ReferenceBlockStore ReferenceKind = 6
)

// TypedReference is one typed dependency or embedded identity.
type TypedReference struct {
	Kind  ReferenceKind
	Value string
}

// ObjectDescriptor is the secret-free metadata required by a migration handler.
type ObjectDescriptor struct {
	ObjectKey         string
	ObjectType        string
	Revision          uint64
	RootDigest        string
	LogicalBytes      uint64
	LogicalBytesKnown bool
	// World is the read-only source World used for typed payload inspection.
	World world.WorldState
	// BlockStoreID is the source block-store identity from the object root.
	BlockStoreID string
	// GraphReferences are graph IRIs attached to this object by the read-only scan.
	GraphReferences       []string
	Dependencies          []string
	References            []TypedReference
	ExternalReferences    []string
	NestedSharedObjectIDs []string
	CanvasNodeIDs         []string
}

// Inspection is the typed closure and disclosure result from a handler.
type Inspection struct {
	Dependencies       []string
	References         []TypedReference
	ExternalReferences []string
}

// RewriteResult contains a rewritten typed payload and graph identities.
type RewriteResult struct {
	// Payload is the serialized replacement root payload. It is never sourced
	// from ObjectDescriptor metadata.
	Payload []byte
	// References is the typed reference view of Payload.
	References []TypedReference
	// GraphReferences is the rewritten graph view owned by this payload.
	GraphReferences []string
}

// Handler classifies an ObjectType and owns its typed reference semantics.
type Handler interface {
	TypeID() string
	Classification() Classification
	Inspect(context.Context, *ObjectDescriptor) (*Inspection, error)
	Rewrite(context.Context, *ObjectDescriptor, *IdentityMap) (*RewriteResult, error)
}
