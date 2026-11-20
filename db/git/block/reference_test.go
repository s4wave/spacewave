package git_block

import (
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
)

// TestValidateReferenceName checks that reference names follow Git's
// check-ref-format rules, one-level names included.
func TestValidateReferenceName(t *testing.T) {
	names := []struct {
		name  string
		valid bool
	}{
		{"main", true},
		{"82dfc39412183d35683e5abb140216efc1d8c802", true},
		{"HEAD", true},
		{"refs/tags/2009.02_rc1", true},
		{"refs/heads/feature+x", true},
		{"this&is&#valid", true},
		{"", false},
		{"@", false},
		{"refs/heads/a..b", false},
		{"refs/heads/x.lock", false},
		{"refs/heads/a b", false},
		{"refs/heads/", false},
		{"/refs/heads/main", false},
		{"refs/tags/v1~1", false},
	}
	for _, v := range names {
		err := ValidateRefName(v.name)
		if v.valid {
			if err != nil {
				t.Fatalf("ref name %s: expected valid: %v", v.name, err)
			}
			t.Logf("OK %s", v.name)
		} else {
			if err == nil {
				t.Fatalf("ref name %s: expected invalid", v.name)
			} else {
				t.Logf("OK (expected): %q: %s", v.name, err.Error())
			}
		}
	}
}

// TestValidateSymbolicTarget checks that HEAD may point at the placeholder a
// clone writes before it knows the remote's default branch, and at no other
// name Git refuses.
func TestValidateSymbolicTarget(t *testing.T) {
	targets := []struct {
		target plumbing.ReferenceName
		valid  bool
	}{
		{plumbing.Invalid, true},
		{"refs/heads/main", true},
		{"refs/heads/a..b", false},
	}
	for _, v := range targets {
		ref, err := NewReference(plumbing.NewSymbolicReference(plumbing.HEAD, v.target))
		if err != nil {
			t.Fatal(err)
		}
		if err := ref.Validate(); (err == nil) != v.valid {
			t.Fatalf("HEAD to %s: valid=%v, err=%v", v.target, v.valid, err)
		}
	}
}
