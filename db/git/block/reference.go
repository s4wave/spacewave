package git_block

import (
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/hash"
)

// ValidateReferenceType validates the reference type.
func ValidateReferenceType(rt plumbing.ReferenceType) error {
	switch rt {
	case plumbing.InvalidReference:
	case plumbing.HashReference:
	case plumbing.SymbolicReference:
	default:
		return errors.Errorf("unknown reference type: %d", int(rt))
	}
	return nil
}

// ValidateRefHash validates a reference hash.
func ValidateRefHash(h *hash.Hash) error {
	if h.GetHashType() == 0 || len(h.GetHash()) == 0 {
		return ErrReferenceHashEmpty
	}
	if h.GetHashType() != hash.HashType_HashType_SHA1 {
		return ErrHashTypeInvalid
	}
	if err := h.Validate(); err != nil {
		return err
	}
	return nil
}

// ValidateRefName validates a reference name by Git's check-ref-format
// rules, which go-git implements, so a Repo stores every name Git does, such
// as Buildroot's tag 2009.02_rc1. A one-level name such as HEAD is allowed,
// its single component held to the same rules, as Git's --allow-onelevel does.
func ValidateRefName(name string) error {
	// Refuse an empty name with its own error.
	if name == "" {
		return ErrReferenceNameEmpty
	}

	// Check a one-level name's component as the last of a full name; "@"
	// alone stays invalid, as in Git.
	full := plumbing.ReferenceName(name)
	if name != "@" && !strings.Contains(name, "/") {
		full = "refs/" + full
	}
	if err := full.Validate(); err != nil {
		return errors.Wrapf(ErrReferenceNameInvalid, "%q", name)
	}
	return nil
}
