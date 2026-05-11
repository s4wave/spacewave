package resource_git

import (
	"strings"
	"testing"
)

func TestBoundedDiffPatchLeavesSmallPatchWhole(t *testing.T) {
	patch := "diff --git a/a.txt b/a.txt\n@@ -1 +1 @@\n-old\n+new\n"
	got, truncated, totalBytes := boundedDiffPatch(patch)
	if truncated {
		t.Fatal("small patch should not be truncated")
	}
	if got != patch {
		t.Fatalf("patch = %q, want %q", got, patch)
	}
	if totalBytes != uint64(len(patch)) {
		t.Fatalf("total bytes = %d, want %d", totalBytes, len(patch))
	}
}

func TestBoundedDiffPatchTruncatesAtLineBoundary(t *testing.T) {
	prefix := "diff --git a/a.txt b/a.txt\n@@ -1 +1 @@\n"
	patch := prefix + strings.Repeat("+0123456789\n", maxDiffPatchBytes/11+2)

	got, truncated, totalBytes := boundedDiffPatch(patch)
	if !truncated {
		t.Fatal("large patch should be truncated")
	}
	if len(got) > maxDiffPatchBytes {
		t.Fatalf("bounded patch length = %d, want <= %d", len(got), maxDiffPatchBytes)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("bounded patch should end on a line boundary: %q", got[len(got)-16:])
	}
	if totalBytes != uint64(len(patch)) {
		t.Fatalf("total bytes = %d, want %d", totalBytes, len(patch))
	}
}
