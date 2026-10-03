package confparse

import "testing"

// TestParseRegexp tests parsing with the regexp package.
func TestParseRegexp(t *testing.T) {
	// Parse a pattern for checking matching and nonmatching text.
	re, err := ParseRegexp("testing .*")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Require a compiled regular expression for the configured pattern.
	if re == nil {
		t.Fail()
	}

	// Verify the regular expression accepts matching text.
	if !re.MatchString("testing 1234") {
		t.Fail()
	}

	// Verify the regular expression rejects unrelated text.
	if re.MatchString("foo bar") {
		t.Fail()
	}
}
