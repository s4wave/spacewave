package git_block

import (
	"regexp"
	"strings"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/pkg/errors"
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

// reValidRef is used to constrain allowed ref names.
// see check-ref-format
// https://github.com/libgit2/libgit2/blob/29715d/src/refs.c#L879
// extended perl regex not supported by Go
// instead, check simplified regex (less permissive)
// note: additional constraints are added in ValidateRefName.
var reValidRef = regexp.MustCompile(`^([a-zA-Z0-9\-\/\.])+$`)

// ^(?!/|.*([/.]\.|//|@\{|\\\\))[^\040\177 ~^:?*\[]+(?<!\.lock|[/.])$

// ValidateRefName validates a reference name.
func ValidateRefName(name string, allowOneLevel bool) error {
	// ref name cannot be empty
	if len(name) == 0 {
		return ErrReferenceNameEmpty
	}
	// constrain to a limited subset of allowed characters
	invRef := !reValidRef.MatchString(name) ||
		// ref name cannot end with "."
		strings.HasSuffix(name, ".") ||
		// ref name cannot end with "/"
		strings.HasSuffix(name, "/")
	// allow only one level if configured
	if invRef || (allowOneLevel && strings.Contains(name, "/")) {
		return ErrReferenceNameInvalid
	}
	return nil
}
