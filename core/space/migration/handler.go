package space_migration

import (
	"context"
	"slices"

	"github.com/pkg/errors"
)

// SchemaInspector decodes one registered built-in payload and returns only
// secret-free typed references. It must not open nested Secret payloads.
type SchemaInspector func(context.Context, *ObjectDescriptor) (*Inspection, error)

// SchemaRewriter decodes, rewrites, and serializes one registered payload.
type SchemaRewriter func(context.Context, *ObjectDescriptor, *IdentityMap) (*RewriteResult, error)

// TypedHandler is a built-in handler with explicit reference-field ownership.
type TypedHandler struct {
	typeID              string
	classification      Classification
	objectKeys          bool
	nestedSharedObjects bool
	canvasNodes         bool
	graphIRIs           bool
	blockStores         bool
	inspectSchema       SchemaInspector
	rewriteSchema       SchemaRewriter
}

// NewSchemaHandler constructs a handler that decodes and rewrites its actual
// world payload. A missing rewriter is an intentional typed refusal, not a
// license to consume descriptor metadata.
func NewSchemaHandler(typeID string, classification Classification, objectKeys, nestedSharedObjects, canvasNodes, graphIRIs, blockStores bool, inspect SchemaInspector, rewriters ...SchemaRewriter) *TypedHandler {
	var rewrite SchemaRewriter
	if len(rewriters) != 0 {
		rewrite = rewriters[0]
	}
	return &TypedHandler{typeID: typeID, classification: classification, objectKeys: objectKeys, nestedSharedObjects: nestedSharedObjects, canvasNodes: canvasNodes, graphIRIs: graphIRIs, blockStores: blockStores, inspectSchema: inspect, rewriteSchema: rewrite}
}

// NewSchemaRefusalHandler registers a built-in whose payload is deliberately
// outside the Phase 0 schema boundary. Planning reports a typed refusal
// instead of consuming synthetic descriptor metadata.
func NewSchemaRefusalHandler(typeID string, classification Classification, detail string) *TypedHandler {
	return NewSchemaHandler(typeID, classification, false, false, false, false, false, func(context.Context, *ObjectDescriptor) (*Inspection, error) {
		return nil, errors.Wrapf(ErrPayloadSchemaRefused, "%s: %s", typeID, detail)
	}, func(context.Context, *ObjectDescriptor, *IdentityMap) (*RewriteResult, error) {
		return nil, errors.Wrapf(ErrPayloadSchemaRefused, "%s: %s", typeID, detail)
	})
}

// TypeID returns the ObjectType identifier owned by this handler.
func (h *TypedHandler) TypeID() string { return h.typeID }

// Classification returns the migration classification.
func (h *TypedHandler) Classification() Classification { return h.classification }

func (h *TypedHandler) sourceInspection(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	if object == nil || object.World == nil {
		return nil, errors.Wrapf(ErrPayloadSchemaRefused, "type %s requires a live read-only World for schema inspection", h.typeID)
	}
	if h.inspectSchema == nil {
		return nil, errors.Wrapf(ErrPayloadSchemaRefused, "type %s has no admitted payload schema", h.typeID)
	}
	inspection, err := h.inspectSchema(ctx, object)
	if err != nil {
		return nil, err
	}
	if inspection == nil {
		inspection = &Inspection{}
	}
	if h.graphIRIs {
		for _, value := range object.GraphReferences {
			if value != "" {
				inspection.References = append(inspection.References, TypedReference{Kind: ReferenceGraphIRI, Value: value})
			}
		}
	}
	return inspection, nil
}

// Inspect returns typed dependencies and external disclosures from the actual
// payload, never from caller-provided metadata.
func (h *TypedHandler) Inspect(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	if object == nil {
		return nil, errors.New("migration object is required")
	}
	inspection, err := h.sourceInspection(ctx, object)
	if err != nil {
		return nil, err
	}
	for _, reference := range inspection.References {
		switch reference.Kind {
		case ReferenceObjectKey:
			if !h.objectKeys {
				return nil, errors.Errorf("type %s has an undeclared object-key reference", h.typeID)
			}
			if reference.Value != "" {
				inspection.Dependencies = append(inspection.Dependencies, reference.Value)
			}
		case ReferenceNestedSharedObject:
			if !h.nestedSharedObjects {
				return nil, errors.Errorf("type %s has an undeclared nested SharedObject reference", h.typeID)
			}
		case ReferenceCanvasNode:
			if !h.canvasNodes {
				return nil, errors.Errorf("type %s has an undeclared Canvas node reference", h.typeID)
			}
		case ReferenceGraphIRI:
			if !h.graphIRIs {
				return nil, errors.Errorf("type %s has an undeclared graph IRI reference", h.typeID)
			}
		case ReferenceExternal:
			if reference.Value != "" {
				inspection.ExternalReferences = append(inspection.ExternalReferences, reference.Value)
			}
		case ReferenceBlockStore:
			if !h.blockStores {
				return nil, errors.Errorf("type %s has an undeclared block-store reference", h.typeID)
			}
			if reference.Value == "" {
				return nil, errors.Errorf("type %s has an empty block-store reference", h.typeID)
			}
		default:
			return nil, errors.Errorf("type %s has unknown reference kind %d", h.typeID, reference.Kind)
		}
	}
	slices.Sort(inspection.Dependencies)
	inspection.Dependencies = slices.Compact(inspection.Dependencies)
	slices.Sort(inspection.ExternalReferences)
	inspection.ExternalReferences = slices.Compact(inspection.ExternalReferences)
	return inspection, nil
}

// Rewrite returns a serialized typed payload and any graph identities owned by
// it. Descriptor-side synthetic references are intentionally ignored.
func (h *TypedHandler) Rewrite(ctx context.Context, object *ObjectDescriptor, mapping *IdentityMap) (*RewriteResult, error) {
	if object == nil || mapping == nil {
		return nil, errors.New("migration object and identity map are required")
	}
	if object.World == nil {
		return nil, errors.Wrapf(ErrPayloadSchemaRefused, "type %s requires a live read-only World for payload rewrite", h.typeID)
	}
	if h.classification == ClassificationUnclassified || h.classification == ClassificationNonMigratable {
		return nil, errors.Errorf("type %s cannot be rewritten with classification %d", h.typeID, h.classification)
	}
	if h.rewriteSchema == nil {
		return nil, errors.Wrapf(ErrPayloadSchemaRefused, "type %s has no admitted payload rewriter", h.typeID)
	}
	result, err := h.rewriteSchema(ctx, object, mapping)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.Wrapf(ErrPayloadSchemaRefused, "type %s returned no rewritten payload", h.typeID)
	}
	return result, nil
}
