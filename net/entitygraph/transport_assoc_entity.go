package bifrost_entitygraph

import (
	"github.com/aperturerobotics/entitygraph/entity"
	el "github.com/aperturerobotics/entitygraph/link"
)

// TransportAssocEntityTypeName is the entitygraph type name for a Bifrost transport
const TransportAssocEntityTypeName = "bifrost/transport/assoc"

// TransportAssocEntity is a entity implementation backed by a transport.
type TransportAssocEntity struct {
	entityID         string
	edgeFrom, edgeTo entity.Ref
}

// GetEntityID returns the entity identifier.
func (l *TransportAssocEntity) GetEntityID() string {
	return l.entityID
}

// GetEntityTypeName returns the entity type name.
func (l *TransportAssocEntity) GetEntityTypeName() string {
	return TransportAssocEntityTypeName
}

// GetEdgeFrom returns the reference to the entity this link starts at.
func (l *TransportAssocEntity) GetEdgeFrom() entity.Ref {
	return l.edgeFrom
}

// GetEdgeTo returns the reference to the entity this link ends at.
func (l *TransportAssocEntity) GetEdgeTo() entity.Ref {
	return l.edgeTo
}

// _ is a type assertion
var _ entity.Entity = ((*TransportAssocEntity)(nil))

// _ is a type assertion
var _ el.Link = ((*TransportAssocEntity)(nil))
