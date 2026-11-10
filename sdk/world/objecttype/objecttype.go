package objecttype

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// ErrWorldStateRequired is returned when the factory requires a WorldState but it is nil.
var ErrWorldStateRequired = errors.New("world state is required")

// ObjectTypeFactory creates a typed srpc.Invoker from an object key.
//
// ctx is the context for the factory operation.
// objectKey is the key of the object to create the typed resource for.
// ws is the WorldState containing this object (may be nil).
// engine is the world Engine for creating write transactions (may be nil).
//
// Returns the invoker, a cleanup function, and any error.
// The cleanup function must be called when the resource is no longer needed.
type ObjectTypeFactory = func(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error)

// ObjectType provides a factory for creating typed resources from objects.
type ObjectType interface {
	// GetObjectTypeID returns the type identifier this factory handles.
	// Format: "alpha/object-layout" or similar.
	//
	// The type ID is also the block type ID of the object's root block, so
	// LookupBlockType with it returns the root's constructor. A reader that
	// decodes bodies or walks refs, such as the body handler and refwalk,
	// relies on that identity, and a type that registers no block type under
	// its ID leaves its objects' roots opaque to them.
	GetObjectTypeID() string
	// GetFactory returns the factory function for creating typed resources.
	GetFactory() ObjectTypeFactory
}

// BodyMarshaler is an ObjectType whose owner renders its object bodies as
// JSON, such as a plugin type the host cannot decode.
type BodyMarshaler interface {
	// MarshalBodiesJSON renders root block bodies as JSON, preserving order.
	// An empty string marks a body the owner could not render.
	MarshalBodiesJSON(ctx context.Context, bodies [][]byte) ([]string, error)
}

// objectType implements ObjectType using generics.
type objectType struct {
	typeID  string
	factory ObjectTypeFactory
}

// NewObjectType creates a new ObjectType with the given type ID and factory.
func NewObjectType(typeID string, factory ObjectTypeFactory) ObjectType {
	return &objectType{
		typeID:  typeID,
		factory: factory,
	}
}

// GetObjectTypeID returns the type identifier this factory handles.
func (t *objectType) GetObjectTypeID() string {
	return t.typeID
}

// GetFactory returns the factory function for creating typed resources.
func (t *objectType) GetFactory() ObjectTypeFactory {
	return t.factory
}

var _ ObjectType = (*objectType)(nil)
