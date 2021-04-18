package git

import (
	"testing"
)

// TestValidateReferenceName tests validating the reference name.
func TestValidateReferenceName(t *testing.T) {
	names := []struct {
		name  string
		valid bool
	}{
		{"main", true},
		{"82dfc39412183d35683e5abb140216efc1d8c802", true},
		{"this&not&#valid", false},
		{"", false},
	}
	for _, v := range names {
		err := ValidateRefName(v.name, false)
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
