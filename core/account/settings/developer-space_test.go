package account_settings

import (
	"errors"
	"testing"
)

// TestSetDeveloperSpace checks that SetDeveloperSpace replaces the recorded
// Space only when it names that Space as previous.
func TestSetDeveloperSpace(t *testing.T) {
	s := &AccountSettings{}
	set := func(id, previous string) error {
		return applyOp(t, s, &AccountSettingsOp{Op: &AccountSettingsOp_SetDeveloperSpace{
			SetDeveloperSpace: &SetDeveloperSpaceOp{SpaceId: id, PreviousSpaceId: previous},
		}})
	}

	// An empty id fails.
	if err := set("", ""); err == nil {
		t.Fatal("expected an empty space id to fail")
	}

	// The first Space is recorded and recording it again changes nothing.
	if err := set("first", ""); err != nil {
		t.Fatal(err)
	}
	if err := set("first", ""); err != nil {
		t.Fatal(err)
	}

	// A racing writer that observed no Space loses to the first.
	if err := set("second", ""); !errors.Is(err, ErrDeveloperSpaceChanged) {
		t.Fatalf("expected ErrDeveloperSpaceChanged, got %v", err)
	}
	if id := s.GetDeveloperSpaceId(); id != "first" {
		t.Fatalf("expected the first developer space, got %q", id)
	}

	// A writer that observed the recorded Space replaces it, and a second
	// writer replacing the same Space loses.
	if err := set("second", "first"); err != nil {
		t.Fatal(err)
	}
	if err := set("third", "first"); !errors.Is(err, ErrDeveloperSpaceChanged) {
		t.Fatalf("expected ErrDeveloperSpaceChanged, got %v", err)
	}
	if id := s.GetDeveloperSpaceId(); id != "second" {
		t.Fatalf("expected the second developer space, got %q", id)
	}
}
