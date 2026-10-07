package account_settings

import (
	"errors"
	"testing"
)

// TestSetDeveloperSpace checks that the first developer Space recorded wins.
func TestSetDeveloperSpace(t *testing.T) {
	s := &AccountSettings{}
	set := func(id string) error {
		return applyOp(t, s, &AccountSettingsOp{Op: &AccountSettingsOp_SetDeveloperSpace{
			SetDeveloperSpace: &SetDeveloperSpaceOp{SpaceId: id},
		}})
	}

	// An empty id fails.
	if err := set(""); err == nil {
		t.Fatal("expected an empty space id to fail")
	}

	// The first Space is recorded and recording it again changes nothing.
	if err := set("first"); err != nil {
		t.Fatal(err)
	}
	if err := set("first"); err != nil {
		t.Fatal(err)
	}

	// A different Space loses to the first.
	if err := set("second"); !errors.Is(err, ErrDeveloperSpaceSet) {
		t.Fatalf("expected ErrDeveloperSpaceSet, got %v", err)
	}
	if id := s.GetDeveloperSpaceId(); id != "first" {
		t.Fatalf("expected the first developer space, got %q", id)
	}
}
