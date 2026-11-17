package s4wave_space

import "testing"

// TestParseObjectURITable pins ParseObjectURI output. The cases must match
// sdk/space/object-uri.test.ts exactly so both runtimes parse identically.
func TestParseObjectURITable(t *testing.T) {
	cases := []struct {
		uri       string
		objectKey string
		path      string
	}{
		{"some/object/key", "some/object/key", ""},
		{"some/object/key/-/foo/bar", "some/object/key", "foo/bar"},
		{"some/object/key/-", "some/object/key", ""},
		{"key/-/", "key", ""},
		{"key/-/foo", "key", "foo"},
		{"-/foo", "foo", ""},
		{"-/-/foo", "-/foo", ""},
		{"-", "", ""},
		{"-/", "", ""},
	}
	for _, c := range cases {
		got := ParseObjectURI(c.uri)
		if got.ObjectKey != c.objectKey || got.Path != c.path {
			t.Errorf("ParseObjectURI(%q) = {%q %q}, want {%q %q}", c.uri, got.ObjectKey, got.Path, c.objectKey, c.path)
		}
	}
}

// TestParseSpacewaveURIEscapes checks that escaped keys the structure would
// otherwise clean or split come back whole. The cases must match the
// escapeObjectKey table in sdk/space/object-uri.test.ts, which writes these
// addresses from these keys, so the writer and the reader stay inverse.
func TestParseSpacewaveURIEscapes(t *testing.T) {
	cases := []struct {
		uri string
		key string
	}{
		{"/u/1/so/sp/-/My%20Repo", "My Repo"},
		{"/u/1/so/sp/-/100%25", "100%"},
		{"/u/1/so/sp/-/repos/caf%C3%A9", "repos/café"},
		{"/u/1/so/sp/-/%2Frepos/x", "/repos/x"},
		{"/u/1/so/sp/-/repos/x%2F", "repos/x/"},
		{"/u/1/so/sp/-/a%2F%2Fb", "a//b"},
		{"/u/1/so/sp/-/a/%2E/b", "a/./b"},
		{"/u/1/so/sp/-/a/%2E%2E/b", "a/../b"},
		{"/u/1/so/sp/-/a/%2D/b", "a/-/b"},
		{"/u/1/so/sp/-/%2D", "-"},
	}
	for _, c := range cases {
		got, err := ParseSpacewaveURI(c.uri)
		if err != nil {
			t.Errorf("ParseSpacewaveURI(%q): %v", c.uri, err)
			continue
		}
		if got.SpaceID != "sp" || len(got.Segments) != 1 || got.Segments[0] != c.key {
			t.Errorf("ParseSpacewaveURI(%q) = %+v, want key %q", c.uri, got, c.key)
		}
	}
	// A part that is not valid percent-encoding reads as written.
	got, err := ParseSpacewaveURI("/u/1/so/sp/-/100%/-/a%zz")
	if err != nil || len(got.Segments) != 2 || got.Segments[0] != "100%" || got.Segments[1] != "a%zz" {
		t.Errorf("ParseSpacewaveURI of raw percent signs = %+v, %v", got, err)
	}
}
