package release

import "testing"

// TestNativeReleaseWithoutBrowserShell permits desktop-only channels while
// rejecting a supplied but incomplete browser shell.
func TestNativeReleaseWithoutBrowserShell(t *testing.T) {
	// Native releases carry their executable through the existing manifest refs.
	metadata := testReleaseMetadata(testBlockRef())
	metadata.ManifestRefs[0].Meta.ManifestId = "desktop"
	metadata.ManifestRefs[0].Meta.PlatformId = "desktop/windows/amd64"
	metadata.BrowserShell = nil
	if err := metadata.Validate(); err != nil {
		t.Fatal(err)
	}

	// Optional metadata remains a complete contract whenever it is supplied.
	metadata.BrowserShell = &BrowserShellMetadata{}
	if err := metadata.Validate(); err == nil {
		t.Fatal("incomplete browser shell accepted")
	}
}
