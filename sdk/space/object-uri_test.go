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
