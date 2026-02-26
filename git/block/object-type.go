package git_block

import "github.com/go-git/go-git/v5/plumbing"

// EncodedObjectType_EncodedObjectType_MAX is the maximum value.
const EncodedObjectType_EncodedObjectType_MAX = EncodedObjectType_EncodedObjectType_REF_DELTA

// NewEncodedObjectType constructs the EncodedObjectType from plumbing type.
func NewEncodedObjectType(ot plumbing.ObjectType) EncodedObjectType {
	// direct mapping
	return EncodedObjectType(ot)
}

// ToObjectType converts the EncodedObjectType to a ObjectType.
func (t EncodedObjectType) ToObjectType() plumbing.ObjectType {
	switch t {
	case EncodedObjectType_EncodedObjectType_COMMIT:
		return plumbing.CommitObject
	case EncodedObjectType_EncodedObjectType_TREE:
		return plumbing.TreeObject
	case EncodedObjectType_EncodedObjectType_BLOB:
		return plumbing.BlobObject
	case EncodedObjectType_EncodedObjectType_TAG:
		return plumbing.TagObject
	case EncodedObjectType_EncodedObjectType_OFS_DELTA:
		return plumbing.OFSDeltaObject
	case EncodedObjectType_EncodedObjectType_REF_DELTA:
		return plumbing.REFDeltaObject
	case 5:
		// no-op
		return plumbing.ObjectType(t) //nolint:gosec
	default:
		return plumbing.InvalidObject
	}
}
