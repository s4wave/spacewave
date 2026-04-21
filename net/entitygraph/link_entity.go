package bifrost_entitygraph

import (
	"strconv"

	"github.com/aperturerobotics/entitygraph/entity"
	el "github.com/aperturerobotics/entitygraph/link"
	"github.com/s4wave/spacewave/net/link"
)

// LinkEntityTypeName is the entitygraph type name for a Bifrost link
const LinkEntityTypeName = "bifrost/link"

// LinkEntity is a entity implementation backed by a link.
type LinkEntity struct {
	link link.Link

	entityID, entityTypeName string
	edgeFrom, edgeTo         entity.Ref
}

// NewLinkEntityRef constructs a new entity ref to a link.
func NewLinkEntityRef(linkUUID uint64) entity.Ref {
	id := strconv.FormatUint(linkUUID, 10)
	return entity.NewEntityRefWithID(id, LinkEntityTypeName)
}

// NewLinkEntity constructs a new LinkEntity
func NewLinkEntity(lnk link.Link) *LinkEntity {
	ref := NewLinkEntityRef(lnk.GetUUID())
	return &LinkEntity{
		link:           lnk,
		entityID:       ref.GetEntityRefId(),
		entityTypeName: LinkEntityTypeName,
		edgeFrom:       NewTransportEntityRef(lnk.GetTransportUUID()),
		edgeTo:         NewTransportEntityRef(lnk.GetRemoteTransportUUID()),
	}
}

// GetEntityID returns the entity identifier.
func (l *LinkEntity) GetEntityID() string {
	return l.entityID
}

// GetEntityTypeName returns the entity type name.
func (l *LinkEntity) GetEntityTypeName() string {
	return l.entityTypeName
}

// GetEdgeFrom returns the reference to the entity this link starts at.
func (l *LinkEntity) GetEdgeFrom() entity.Ref {
	return l.edgeFrom
}

// GetEdgeTo returns the reference to the entity this link ends at.
func (l *LinkEntity) GetEdgeTo() entity.Ref {
	return l.edgeTo
}

// _ is a type assertion
var _ entity.Entity = ((*LinkEntity)(nil))

// _ is a type assertion
var _ el.Link = ((*LinkEntity)(nil))
