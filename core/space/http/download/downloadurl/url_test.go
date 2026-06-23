package downloadurl

import "testing"

func TestParseGitRepoMetadataPath(t *testing.T) {
	req, err := Parse("/fs/u/4/so/space-download/-/repo/demo/-/HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if req.SessionIdx != 4 {
		t.Fatalf("sessionIdx: got %d, want 4", req.SessionIdx)
	}
	if req.SharedObjectID != "space-download" {
		t.Fatalf("sharedObjectID: got %q, want %q", req.SharedObjectID, "space-download")
	}
	if req.ProjectedPath != "u/4/so/space-download/-/repo/demo/-/HEAD" {
		t.Fatalf("projectedPath: got %q", req.ProjectedPath)
	}
}

func TestParseDecodesProjectedFileSegments(t *testing.T) {
	req, err := Parse("/fs/u/1/so/space-download/-/files/-/what%20is%20this.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if req.ProjectedPath != "u/1/so/space-download/-/files/-/what is this.mp4" {
		t.Fatalf("projectedPath: got %q", req.ProjectedPath)
	}
}
