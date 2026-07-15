package space_migration

import (
	"context"
	"slices"

	"github.com/pkg/errors"
)

// ErrHandlerMissing identifies an ObjectType with no migration classification.
var ErrHandlerMissing = errors.New("migration handler is missing")

// Registry owns the migration classification for registered ObjectTypes.
type Registry struct {
	handlers map[string]Handler
}

// NewRegistry constructs an empty migration registry.
func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]Handler)}
}

// Register adds one handler and rejects duplicate ObjectType registrations.
func (r *Registry) Register(handler Handler) error {
	if r == nil {
		return errors.New("migration registry is required")
	}
	if handler == nil || handler.TypeID() == "" {
		return errors.New("migration handler type id is required")
	}
	if !validClassification(handler.Classification()) {
		return errors.Errorf("migration handler classification %d is invalid", handler.Classification())
	}
	if _, exists := r.handlers[handler.TypeID()]; exists {
		return errors.Errorf("migration handler already registered for %s", handler.TypeID())
	}
	r.handlers[handler.TypeID()] = handler
	return nil
}

// Lookup returns the handler for an ObjectType, or nil when unclassified.
func (r *Registry) Lookup(typeID string) Handler {
	if r == nil {
		return nil
	}
	return r.handlers[typeID]
}

// TypeIDs returns all registered ObjectTypes in lexical order.
func (r *Registry) TypeIDs() []string {
	if r == nil {
		return nil
	}
	ids := make([]string, 0, len(r.handlers))
	for typeID := range r.handlers {
		ids = append(ids, typeID)
	}
	slices.Sort(ids)
	return ids
}

// Inspect runs the registered handler for one object.
func (r *Registry) Inspect(ctx context.Context, object *ObjectDescriptor) (*Inspection, error) {
	if object == nil {
		return nil, errors.New("migration object is required")
	}
	handler := r.Lookup(object.ObjectType)
	if handler == nil {
		return nil, errors.Wrapf(ErrHandlerMissing, "type %s object %s", object.ObjectType, object.ObjectKey)
	}
	return handler.Inspect(ctx, object)
}
