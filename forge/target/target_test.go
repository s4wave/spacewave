package forge_target

import (
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block/sbset"
)

// TestValidateRejectsDuplicateInputNames checks that a Target with two inputs
// of the same name fails validation.
func TestValidateRejectsDuplicateInputNames(t *testing.T) {
	tgt := &Target{Inputs: []*Input{{Name: "src"}, {Name: "src"}}}
	if err := tgt.Validate(); !errors.Is(err, sbset.ErrNonUniqueName) {
		t.Fatalf("Validate() = %v, want %v", err, sbset.ErrNonUniqueName)
	}
}
