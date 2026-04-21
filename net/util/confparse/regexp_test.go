package confparse

import "testing"

// TestParseRegexp tests parsing with the regexp package.
func TestParseRegexp(t *testing.T) {
	re, err := ParseRegexp("testing .*")
	if err != nil {
		t.Fatal(err.Error())
	}
	if re == nil {
		t.Fail()
	}
	if !re.MatchString("testing 1234") {
		t.Fail()
	}
	if re.MatchString("foo bar") {
		t.Fail()
	}
}
