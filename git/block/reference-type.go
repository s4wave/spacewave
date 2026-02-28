package git_block

import "github.com/go-git/go-git/v6/plumbing"

// ReferenceType_ReferenceType_MAX is the maximum value.
const ReferenceType_ReferenceType_MAX = ReferenceType_ReferenceType_SYMBOLIC

// NewReferenceType constructs the ReferenceType from plumbing type.
func NewReferenceType(ot plumbing.ReferenceType) ReferenceType {
	// direct mapping
	return ReferenceType(ot)
}

// ToReferenceType converts the ReferenceType to a ReferenceType.
func (t ReferenceType) ToReferenceType() plumbing.ReferenceType {
	switch t {
	case ReferenceType_ReferenceType_SYMBOLIC:
		return plumbing.SymbolicReference
	case ReferenceType_ReferenceType_HASH:
		return plumbing.HashReference
	default:
		return plumbing.InvalidReference
	}
}
