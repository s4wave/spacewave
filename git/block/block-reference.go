package git_block

import (
	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/pkg/errors"
)

// NewReference constructs a new repo ref.
func NewReference(ref *plumbing.Reference) (*Reference, error) {
	if ref == nil || len(ref.Name()) == 0 {
		return nil, ErrReferenceNameEmpty
	}
	var refHash *hash.Hash
	var err error
	if ref.Type() == plumbing.HashReference {
		rh := ref.Hash()
		refHash, err = NewHash(rh)
		if err != nil {
			return nil, err
		}
	}
	return &Reference{
		Hash:                refHash,
		Name:                string(ref.Name()),
		ReferenceType:       NewReferenceType(ref.Type()),
		TargetReferenceName: string(ref.Target()),
	}, nil
}

// NewReferenceBlock builds a new repo ref block.
func NewReferenceBlock() block.Block {
	return &Reference{}
}

// IsNil returns if the object is nil.
func (r *Reference) IsNil() bool {
	return r == nil
}

// Validate checks the reference.
func (r *Reference) Validate() error {
	if err := ValidateRefName(r.GetName(), false); err != nil {
		return err
	}
	rt := plumbing.ReferenceType(r.GetReferenceType()) //nolint:gosec
	if err := ValidateReferenceType(rt); err != nil {
		return err
	}
	if rt == plumbing.HashReference {
		if err := ValidateRefHash(r.GetHash()); err != nil {
			return err
		}
	} else {
		if len(r.GetHash().GetHash()) != 0 || r.GetHash().GetHashType() != 0 {
			// make sure no extra data is hidden here
			return errors.New("reference hash field filled for non-hash ref type")
		}
	}
	if rt == plumbing.SymbolicReference {
		if err := ValidateRefName(r.GetTargetReferenceName(), false); err != nil {
			return errors.Wrap(err, "symbolic ref")
		}
	} else {
		if len(r.GetTargetReferenceName()) != 0 {
			return errors.New("expected empty target_reference_name field")
		}
	}
	return nil
}

// ToReference converts to a plumbing reference.
func (r *Reference) ToReference() (*plumbing.Reference, error) {
	if len(r.GetName()) == 0 {
		return nil, ErrReferenceNameEmpty
	}
	switch r.GetReferenceType() {
	case ReferenceType_ReferenceType_SYMBOLIC:
		return plumbing.NewSymbolicReference(
			plumbing.ReferenceName(r.GetName()),
			plumbing.ReferenceName(r.GetTargetReferenceName()),
		), nil
	case ReferenceType_ReferenceType_HASH:
		if len(r.GetHash().GetHash()) == 0 {
			return nil, ErrReferenceHashEmpty
		}
		h, err := FromHash(r.GetHash())
		if err != nil {
			return nil, err
		}
		return plumbing.NewHashReference(
			plumbing.ReferenceName(r.GetName()),
			h,
		), nil
	default:
		return nil, errors.Wrap(
			ErrReferenceTypeInvalid,
			r.GetReferenceType().String(),
		)
	}
}

// MarshalBlock marshals the block to binary.
func (r *Reference) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (r *Reference) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// _ is a type assertion
var (
	_ block.Block         = ((*Reference)(nil))
	_ sbset.NamedSubBlock = ((*Reference)(nil))
)
