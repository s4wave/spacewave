package bifrost_entitygraph

import (
	"strconv"

	"github.com/aperturerobotics/entitygraph/entity"
	"github.com/s4wave/spacewave/net/peer"
)

// TransportEntityTypeName is the entitygraph type name for a Bifrost transport
const TransportEntityTypeName = "bifrost/transport"

// TransportEntity is a entity implementation backed by a transport.
type TransportEntity struct {
	entityID, entityTypeName string
}

// NewTransportEntityRef constructs a new entity ref to a transport.
func NewTransportEntityRef(transportID uint64) entity.Ref {
	tptIDStr := strconv.FormatUint(transportID, 10)
	return entity.NewEntityRefWithID(tptIDStr, TransportEntityTypeName)
}

// NewTransportEntity constructs a new TransportEntity and TransportAssocEntity.
func NewTransportEntity(tptUUID uint64, peerID peer.ID) (*TransportEntity, *TransportAssocEntity) {
	tptRef := NewTransportEntityRef(tptUUID)
	tptID := tptRef.GetEntityRefId()
	nodeRef := NewPeerEntityRef(peerID)
	return &TransportEntity{
			entityID:       tptID,
			entityTypeName: TransportEntityTypeName,
		}, &TransportAssocEntity{
			entityID: tptID + "-assoc",
			edgeFrom: tptRef,
			edgeTo:   nodeRef,
		}
}

// GetEntityID returns the entity identifier.
func (l *TransportEntity) GetEntityID() string {
	return l.entityID
}

// GetEntityTypeName returns the entity type name.
func (l *TransportEntity) GetEntityTypeName() string {
	return l.entityTypeName
}

// _ is a type assertion
var _ entity.Entity = ((*TransportEntity)(nil))
