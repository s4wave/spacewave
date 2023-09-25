package determine_cjs_exports

import (
	"strings"
	"testing"
)

func TestGetDetermineCjsExportsScript(t *testing.T) {
	exportsScript := GetDetermineCjsExportsScript()
	if !strings.Contains(exportsScript, "enhanced-resolve") {
		t.FailNow()
	}
}

func TestSupportsExtension(t *testing.T) {
	tests := []bool{
		SupportsExtension("test.js"),
		!SupportsExtension("test.png"),
		!SupportsExtension("png"),
		SupportsExtension("js"),
		!SupportsExtension("jpg"),
		!SupportsExtension(".jpg"),
		!SupportsExtension("test.jpg"),
		SupportsExtension(".js"),
		// SupportsExtension(".mjs"),
		SupportsExtension(""),
	}
	for _, tr := range tests {
		if !tr {
			t.Fatalf("tests failed: %v", tests)
		}
	}
}
