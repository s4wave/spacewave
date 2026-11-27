//go:build !js

package spacewave_cli

import "testing"

// TestParseGitRemoteURL checks both remote address forms and that only the
// session-qualified form specifies a session.
func TestParseGitRemoteURL(t *testing.T) {
	cases := []struct {
		url  string
		want fsURI
	}{
		{"spacewave://space-1/cdn/git", fsURI{spaceID: "space-1", objectKey: "cdn/git"}},
		{"space-1/repo", fsURI{spaceID: "space-1", objectKey: "repo"}},
		{"spacewave:///u/3/so/space-1/-/cdn/git", fsURI{sessionIdx: 3, spaceID: "space-1", objectKey: "cdn/git"}},
		{"spacewave:///u/1/so/space-1/-/My%20Repo/%2D/x%2F", fsURI{sessionIdx: 1, spaceID: "space-1", objectKey: "My Repo/-/x/"}},
		{"spacewave://space-1/My%20Repo", fsURI{spaceID: "space-1", objectKey: "My Repo"}},
		{"spacewave://space-1/100%", fsURI{spaceID: "space-1", objectKey: "100%"}},
	}
	for _, tc := range cases {
		got, err := parseGitRemoteURL(tc.url)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.url, err)
		}
		if got != tc.want {
			t.Fatalf("parse %q: got %+v, want %+v", tc.url, got, tc.want)
		}
	}

	for _, url := range []string{
		"spacewave://space-1",
		"spacewave:///u/3/so/space-1",
		"spacewave:///u/3/so/space-1/-/repo/-/dir",
		"spacewave:///u/x/so/space-1/-/repo",
	} {
		if _, err := parseGitRemoteURL(url); err == nil {
			t.Fatalf("parse %q: want an error", url)
		}
	}
}
