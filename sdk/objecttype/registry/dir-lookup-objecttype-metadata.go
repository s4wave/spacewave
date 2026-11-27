package s4wave_objecttype_registry

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// LookupObjectTypeMetadata is a directive to look up the registered metadata
// of an object type, including the ports it declares.
type LookupObjectTypeMetadata interface {
	// Directive indicates LookupObjectTypeMetadata is a directive.
	directive.Directive

	// LookupObjectTypeMetadataTypeID returns the object type ID to look up.
	LookupObjectTypeMetadataTypeID() string

	// LookupObjectTypeMetadataEngineID returns the world engine whose demanded
	// plugins scope the lookup, matching LookupObjectType.
	LookupObjectTypeMetadataEngineID() string
}

// LookupObjectTypeMetadataValue is the result type for
// LookupObjectTypeMetadata: a private copy of the registered metadata. A type
// registered without metadata yields an empty, non-nil value.
type LookupObjectTypeMetadataValue = *ObjectTypeMetadata

// ExLookupObjectTypeMetadata returns the metadata registered for an object type
// on the bus, scoped to the engine in ctx the way ExLookupObjectType is.
//
// Returns nil when no plugin has registered the type, which includes a
// registration that was released. The caller owns the returned copy.
func ExLookupObjectTypeMetadata(
	ctx context.Context,
	b bus.Bus,
	typeID string,
) (*ObjectTypeMetadata, error) {
	av, _, avRef, err := bus.ExecOneOffTyped[LookupObjectTypeMetadataValue](
		ctx,
		b,
		NewLookupObjectTypeMetadata(typeID, objecttype.EngineIDFromContext(ctx)),
		bus.ReturnWhenIdle(),
		nil,
	)
	if err != nil {
		return nil, err
	}
	if av == nil {
		if avRef != nil {
			avRef.Release()
		}
		return nil, nil
	}
	metadata := av.GetValue().CloneVT()
	avRef.Release()
	return metadata, nil
}

// ExWatchObjectTypeMetadata calls onChange with the metadata registered for an
// object type each time the metadata changes, scoped to engineID the way
// ExLookupObjectTypeMetadata is scoped by its context. The registry answers the
// lookup directive for as long as it is held, so the callback runs when a
// plugin registers the type, replaces its registration, or releases it.
// onChange receives nil when no plugin registers the type, and each call gets
// its own private copy of the metadata.
//
// ExWatchObjectTypeMetadata returns once the registry has answered the lookup
// for the first time, so a change after it returns always calls onChange, and
// the registration present at that point has already been delivered.
//
// onChange runs inside the directive's value callback and must return quickly
// without calling back into the bus. Call release to stop watching. onChange is
// not called after release returns.
func ExWatchObjectTypeMetadata(
	ctx context.Context,
	b bus.Bus,
	typeID, engineID string,
	onChange func(metadata *ObjectTypeMetadata),
) (release func(), err error) {
	// Watch the lookup and stop delivering values once released.
	var mtx sync.Mutex
	var released bool
	di, ref, err := bus.ExecOneOffWatchCb(
		ctx,
		func(val directive.TypedAttachedValue[LookupObjectTypeMetadataValue]) bool {
			mtx.Lock()
			defer mtx.Unlock()
			if released {
				return true
			}
			var metadata *ObjectTypeMetadata
			if val != nil {
				metadata = val.GetValue().CloneVT()
			}
			onChange(metadata)
			return true
		},
		b,
		NewLookupObjectTypeMetadata(typeID, engineID),
	)
	if err != nil {
		return nil, err
	}
	release = func() {
		ref.Release()
		mtx.Lock()
		released = true
		mtx.Unlock()
	}

	// Wait for the registry's first answer so no change goes unobserved.
	idle := make(chan struct{})
	var idleOnce sync.Once
	relIdle := di.AddIdleCallback(func(isIdle bool, _ []error) {
		if isIdle {
			idleOnce.Do(func() { close(idle) })
		}
	})
	defer relIdle()
	select {
	case <-idle:
		return release, nil
	case <-ctx.Done():
		release()
		return nil, context.Cause(ctx)
	}
}

// lookupObjectTypeMetadata implements LookupObjectTypeMetadata.
type lookupObjectTypeMetadata struct {
	objectTypeID string
	engineID     string
}

// NewLookupObjectTypeMetadata constructs a LookupObjectTypeMetadata directive
// scoped to the demanded plugins of one world engine. An empty engineID is
// unscoped.
func NewLookupObjectTypeMetadata(objectTypeID, engineID string) LookupObjectTypeMetadata {
	return &lookupObjectTypeMetadata{
		objectTypeID: objectTypeID,
		engineID:     engineID,
	}
}

// Validate validates the directive.
func (d *lookupObjectTypeMetadata) Validate() error {
	return nil
}

// GetValueOptions returns options relating to value handling.
//
// The directive is disposed as soon as it is unreferenced. A lingering
// instance would hand a later one-off lookup the metadata of a registration
// released before the resolver wakes to remove it.
func (d *lookupObjectTypeMetadata) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// LookupObjectTypeMetadataTypeID returns the object type ID to look up.
func (d *lookupObjectTypeMetadata) LookupObjectTypeMetadataTypeID() string {
	return d.objectTypeID
}

// LookupObjectTypeMetadataEngineID returns the world engine scoping the lookup.
func (d *lookupObjectTypeMetadata) LookupObjectTypeMetadataEngineID() string {
	return d.engineID
}

// IsEquivalent checks if the other directive asks for the same type and scope.
func (d *lookupObjectTypeMetadata) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupObjectTypeMetadata)
	if !ok {
		return false
	}
	return d.objectTypeID == od.LookupObjectTypeMetadataTypeID() &&
		d.engineID == od.LookupObjectTypeMetadataEngineID()
}

// Superceeds checks if the directive overrides another.
func (d *lookupObjectTypeMetadata) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
func (d *lookupObjectTypeMetadata) GetName() string {
	return "LookupObjectTypeMetadata"
}

// GetDebugVals returns the directive arguments stringified.
func (d *lookupObjectTypeMetadata) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if d.objectTypeID != "" {
		vals["object-type-id"] = []string{d.objectTypeID}
	}
	if d.engineID != "" {
		vals["engine-id"] = []string{d.engineID}
	}
	return vals
}

var _ LookupObjectTypeMetadata = (*lookupObjectTypeMetadata)(nil)
