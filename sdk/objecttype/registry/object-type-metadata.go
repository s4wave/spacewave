package s4wave_objecttype_registry

import "github.com/s4wave/spacewave/sdk/world/objecttype"

// MetadataObjectType is an ObjectType that carries its own metadata, such as
// the ports a Flowgraph binds it by. The object type controller resolves
// LookupObjectTypeMetadata for such a type from the type itself.
type MetadataObjectType interface {
	objecttype.ObjectType
	// GetObjectTypeMetadata returns the type's metadata. The caller must not
	// modify it.
	GetObjectTypeMetadata() *ObjectTypeMetadata
}

// metadataObjectType is an ObjectType with its metadata attached.
type metadataObjectType struct {
	objecttype.ObjectType
	metadata *ObjectTypeMetadata
}

// WithMetadata attaches metadata to an ObjectType.
func WithMetadata(objType objecttype.ObjectType, metadata *ObjectTypeMetadata) MetadataObjectType {
	return &metadataObjectType{ObjectType: objType, metadata: metadata}
}

// GetObjectTypeMetadata returns the type's metadata.
func (t *metadataObjectType) GetObjectTypeMetadata() *ObjectTypeMetadata {
	return t.metadata
}

// _ is a type assertion.
var _ MetadataObjectType = (*metadataObjectType)(nil)
