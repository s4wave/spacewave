package confparse

import (
	"net/url"
	"testing"
)

// TestURLs tests parsing URLs.
func TestURLs(t *testing.T) {
	fatal := func(u *url.URL, err error) {
		if err != nil {
			t.Fatal(err.Error())
		}
		if u == nil {
			t.Fail()
		}
	}

	fatal(ParseURL("https://test.com"))
	fatal(ParseURL("http://www.google.com"))

	u, err := ParseURL("")
	if err != nil {
		t.Fatal(err.Error())
	}
	if u != nil {
		t.Fail()
	}
}
