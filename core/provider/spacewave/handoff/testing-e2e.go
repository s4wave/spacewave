//go:build e2e

package provider_spacewave_handoff

// SetBrowserOpenerForTesting replaces the desktop browser opener for e2e
// tests and returns a restore function.
func SetBrowserOpenerForTesting(opener func(string) error) func() {
	prev := browserOpener
	browserOpener = opener
	return func() {
		browserOpener = prev
	}
}
