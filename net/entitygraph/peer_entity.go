package bifrost_entitygraph

import (
	"github.com/aperturerobotics/entitygraph/entity"
	"github.com/s4wave/spacewave/net/peer"
)

// PeerEntityTypeName is the entitygraph type name for a Bifrost peer
const PeerEntityTypeName = "bifrost/peer"

// PeerEntity is a entity implementation backed by a node.
type PeerEntity struct {
	entityID, entityTypeName string
}

// NewPeerEntityRef constructs a new entity ref to a node.
func NewPeerEntityRef(peerID peer.ID) entity.Ref {
	return entity.NewEntityRefWithID(peerID.String(), PeerEntityTypeName)
}

// NewPeerEntity constructs a new PeerEntity.
func NewPeerEntity(peerID peer.ID) *PeerEntity {
	nodRef := NewPeerEntityRef(peerID)
	nodID := nodRef.GetEntityRefId()
	return &PeerEntity{
		entityID:       nodID,
		entityTypeName: PeerEntityTypeName,
	}
}

// GetEntityID returns the entity identifier.
func (e *PeerEntity) GetEntityID() string {
	return e.entityID
}

// GetEntityTypeName returns the entity type name.
func (e *PeerEntity) GetEntityTypeName() string {
	return e.entityTypeName
}

var _ entity.Entity = ((*PeerEntity)(nil))
