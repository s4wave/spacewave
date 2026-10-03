package confparse

import (
	"net/url"
	"testing"
)

// TestURLs tests parsing URLs.
func TestURLs(t *testing.T) {
	// Prepare an assertion that requires a parsed URL.
	fatal := func(u *url.URL, err error) {
		// Require URL parsing to succeed.
		if err != nil {
			t.Fatal(err.Error())
		}

		// Require a URL for each nonempty configuration value.
		if u == nil {
			t.Fail()
		}
	}

	// Verify HTTP and HTTPS configuration values produce URLs.
	fatal(ParseURL("https://test.com"))
	fatal(ParseURL("http://www.google.com"))

	// Parse an empty URL configuration value.
	u, err := ParseURL("")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify an empty configuration value leaves the URL absent.
	if u != nil {
		t.Fail()
	}
}
