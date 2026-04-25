package space_http_download

import "testing"

func TestDownloadURLParsingGitRepoMetadataPath(t *testing.T) {
	req, err := parseDownloadURL("/fs/u/4/so/space-download/-/repo/demo/-/HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if req.sessionIdx != 4 {
		t.Fatalf("sessionIdx: got %d, want 4", req.sessionIdx)
	}
	if req.sharedObjectID != "space-download" {
		t.Fatalf("sharedObjectID: got %q, want %q", req.sharedObjectID, "space-download")
	}
	if req.projectedPath != "u/4/so/space-download/-/repo/demo/-/HEAD" {
		t.Fatalf("projectedPath: got %q", req.projectedPath)
	}
}
