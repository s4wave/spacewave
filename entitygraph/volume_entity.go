package hydra_entitygraph

import (
	"github.com/aperturerobotics/entitygraph/entity"
	"github.com/aperturerobotics/hydra/volume"
)

// VolumeEntityTypeName is the entitygraph type name for a Hydra volume.
const VolumeEntityTypeName = "hydra/volume"

// VolumeEntity is a entity implementation backed by a link.
type VolumeEntity struct {
	vol volume.Volume

	entityID, entityTypeName string
	edgeFrom, edgeTo         entity.Ref
}

// NewVolumeEntityRef constructs a new entity ref to a link.
func NewVolumeEntityRef(volumeID string) entity.Ref {
	return entity.NewEntityRefWithID(
		volumeID,
		VolumeEntityTypeName,
	)
}

// NewVolumeEntity constructs a new VolumeEntity
func NewVolumeEntity(vol volume.Volume) *VolumeEntity {
	ref := NewVolumeEntityRef(vol.GetID())
	return &VolumeEntity{
		vol:            vol,
		entityID:       ref.GetEntityRefId(),
		entityTypeName: VolumeEntityTypeName,
	}
}

// GetEntityID returns the entity identifier.
func (l *VolumeEntity) GetEntityID() string {
	return l.entityID
}

// GetEntityTypeName returns the entity type name.
func (l *VolumeEntity) GetEntityTypeName() string {
	return l.entityTypeName
}

// _ is a type assertion
var _ entity.Entity = ((*VolumeEntity)(nil))
