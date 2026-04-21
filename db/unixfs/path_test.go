package unixfs

import (
	"io/fs"
	"testing"
)

func TestSplitPath(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		want       []string
		isAbsolute bool
	}{
		{
			name:       "empty path",
			path:       "",
			want:       nil,
			isAbsolute: false,
		},
		{
			name:       "root path",
			path:       "/",
			want:       nil,
			isAbsolute: true,
		},
		{
			name:       "relative path",
			path:       "foo/bar",
			want:       []string{"foo", "bar"},
			isAbsolute: false,
		},
		{
			name:       "absolute path",
			path:       "/foo/bar",
			want:       []string{"foo", "bar"},
			isAbsolute: true,
		},
		{
			name:       "current directory",
			path:       "./foo/bar",
			want:       []string{"foo", "bar"},
			isAbsolute: false,
		},
		{
			name:       "double slash",
			path:       "foo//bar",
			want:       []string{"foo", "bar"},
			isAbsolute: false,
		},
		{
			name:       "trailing slash",
			path:       "foo/bar/",
			want:       []string{"foo", "bar"},
			isAbsolute: false,
		},
		{
			name:       "multiple dots",
			path:       "./foo/./bar",
			want:       []string{"foo", "bar"},
			isAbsolute: false,
		},
		{
			name:       "special characters",
			path:       "foo-1/bar_2",
			want:       []string{"foo-1", "bar_2"},
			isAbsolute: false,
		},
		{
			name:       "unicode characters",
			path:       "münchen/straße",
			want:       []string{"münchen", "straße"},
			isAbsolute: false,
		},
		{
			name:       "multiple consecutive slashes",
			path:       "foo///bar////baz",
			want:       []string{"foo", "bar", "baz"},
			isAbsolute: false,
		},
		{
			name:       "absolute path with multiple slashes",
			path:       "///foo////bar",
			want:       []string{"foo", "bar"},
			isAbsolute: true,
		},
		{
			name:       "dot path with slashes",
			path:       "./////foo",
			want:       []string{"foo"},
			isAbsolute: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, isAbs := SplitPath(tt.path)
			if isAbs != tt.isAbsolute {
				t.Fatalf("SplitPath(%q) isAbsolute = %v, want %v", tt.path, isAbs, tt.isAbsolute)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("SplitPath(%q) got len = %v, want len = %v", tt.path, len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("SplitPath(%q)[%d] = %v, want %v", tt.path, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCleanSplitValidateRelativePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		want    []string
		wantErr error
	}{
		{
			name:    "empty path",
			path:    "",
			want:    nil,
			wantErr: nil,
		},
		{
			name:    "root path",
			path:    "/",
			want:    nil,
			wantErr: nil,
		},
		{
			name:    "dot path",
			path:    ".",
			want:    nil,
			wantErr: nil,
		},
		{
			name:    "simple relative path",
			path:    "foo/bar",
			want:    []string{"foo", "bar"},
			wantErr: nil,
		},
		{
			name:    "absolute path converted to relative",
			path:    "/foo/bar",
			want:    []string{"foo", "bar"},
			wantErr: nil,
		},
		{
			name:    "path with dot segments",
			path:    "./foo/./bar",
			want:    []string{"foo", "bar"},
			wantErr: nil,
		},
		{
			name:    "invalid path with dot dot",
			path:    "../foo",
			want:    nil,
			wantErr: fs.ErrInvalid,
		},
		{
			name:    "invalid path with internal dot dot",
			path:    "foo/../bar",
			want:    []string{"bar"},
			wantErr: nil,
		},
		{
			name:    "multiple dot dot segments",
			path:    "foo/../../bar",
			want:    nil,
			wantErr: fs.ErrInvalid,
		},
		{
			name:    "complex dot dot path",
			path:    "a/b/../../c/./d/../e",
			want:    []string{"c", "e"},
			wantErr: nil,
		},
		{
			name:    "path with spaces",
			path:    "my documents/some file.txt",
			want:    []string{"my documents", "some file.txt"},
			wantErr: nil,
		},
		{
			name:    "path with special characters",
			path:    "$HOME/#temp/@user",
			want:    []string{"$HOME", "#temp", "@user"},
			wantErr: nil,
		},
		{
			name:    "path with backslashes",
			path:    "foo\\bar/baz",
			want:    []string{"foo\\bar", "baz"},
			wantErr: nil,
		},
		{
			name:    "path with multiple consecutive dots",
			path:    "foo/.../bar",
			want:    []string{"foo", "...", "bar"},
			wantErr: nil,
		},
		{
			name:    "path with dot dot and special chars",
			path:    "../$foo/./~bar/../#baz",
			want:    nil,
			wantErr: fs.ErrInvalid,
		},
		{
			name:    "path with empty components",
			path:    "foo//bar///",
			want:    []string{"foo", "bar"},
			wantErr: nil,
		},
		{
			name:    "path with special characters",
			path:    "foo-1/bar_2",
			want:    []string{"foo-1", "bar_2"},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CleanSplitValidateRelativePath(tt.path)
			if err != tt.wantErr {
				t.Errorf("CleanSplitValidateRelativePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("CleanSplitValidateRelativePath(%q) got len = %v, want len = %v", tt.path, len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("CleanSplitValidateRelativePath(%q)[%d] = %v, want %v", tt.path, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestJoinPath(t *testing.T) {
	tests := []struct {
		name       string
		parts      []string
		isAbsolute bool
		want       string
	}{
		{
			name:       "empty parts",
			parts:      nil,
			isAbsolute: false,
			want:       ".",
		},
		{
			name:       "single part",
			parts:      []string{"foo"},
			isAbsolute: false,
			want:       "foo",
		},
		{
			name:       "multiple parts",
			parts:      []string{"foo", "bar"},
			isAbsolute: false,
			want:       "foo/bar",
		},
		{
			name:       "absolute path",
			parts:      []string{"foo", "bar"},
			isAbsolute: true,
			want:       "/foo/bar",
		},
		{
			name:       "empty strings",
			parts:      []string{"", "foo", "", "bar", ""},
			isAbsolute: false,
			want:       "foo/bar",
		},
		{
			name:       "special characters",
			parts:      []string{"foo-1", "bar_2"},
			isAbsolute: false,
			want:       "foo-1/bar_2",
		},
		{
			name:       "unicode characters",
			parts:      []string{"münchen", "straße"},
			isAbsolute: false,
			want:       "münchen/straße",
		},
		{
			name:       "absolute path with empty components",
			parts:      []string{"", "foo", "", "", "bar", ""},
			isAbsolute: true,
			want:       "/foo/bar",
		},
		{
			name:       "only empty components",
			parts:      []string{"", "", ""},
			isAbsolute: false,
			want:       ".",
		},
		{
			name:       "dot components",
			parts:      []string{".", "foo", ".", "bar", "."},
			isAbsolute: false,
			want:       "foo/bar",
		},
		{
			name:       "empty absolute path",
			parts:      []string{},
			isAbsolute: true,
			want:       "/",
		},
		{
			name:       "path with spaces",
			parts:      []string{"my docs", "some file.txt"},
			isAbsolute: false,
			want:       "my docs/some file.txt",
		},
		{
			name:       "path with special characters",
			parts:      []string{"$home", "#temp", "@user"},
			isAbsolute: false,
			want:       "$home/#temp/@user",
		},
		{
			name:       "path with mixed slashes",
			parts:      []string{"foo/bar", "baz\\qux"},
			isAbsolute: false,
			want:       "foo/bar/baz\\qux",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := JoinPath(tt.parts, tt.isAbsolute)
			if got != tt.want {
				t.Fatalf("JoinPath(%v, %v) = %v, want %v", tt.parts, tt.isAbsolute, got, tt.want)
			}
		})
	}
}

func TestJoinPathPts(t *testing.T) {
	tests := []struct {
		name string
		pts  [][]string
		want []string
	}{
		{
			name: "empty input",
			pts:  nil,
			want: nil,
		},
		{
			name: "single slice",
			pts:  [][]string{{"foo", "bar"}},
			want: []string{"foo", "bar"},
		},
		{
			name: "multiple slices",
			pts:  [][]string{{"foo"}, {"bar", "baz"}},
			want: []string{"foo", "bar", "baz"},
		},
		{
			name: "empty slices",
			pts:  [][]string{{"foo"}, {}, {"bar"}, {"baz"}},
			want: []string{"foo", "bar", "baz"},
		},
		{
			name: "nested paths",
			pts:  [][]string{{"foo", "bar"}, {"baz", "qux"}, {"quux"}},
			want: []string{"foo", "bar", "baz", "qux", "quux"},
		},
		{
			name: "special characters",
			pts:  [][]string{{"foo-1"}, {"bar_2", "baz-3"}},
			want: []string{"foo-1", "bar_2", "baz-3"},
		},
		{
			name: "nil slices",
			pts:  [][]string{nil, {"foo"}, nil, {"bar"}, nil},
			want: []string{"foo", "bar"},
		},
		{
			name: "single empty string",
			pts:  [][]string{{""}},
			want: []string{""},
		},
		{
			name: "mixed empty and non-empty slices",
			pts:  [][]string{{"foo"}, {""}, {"bar", ""}, {"", "baz"}},
			want: []string{"foo", "", "bar", "", "", "baz"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := JoinPathPts(tt.pts...)
			if len(got) != len(tt.want) {
				t.Fatalf("JoinPathPts() got len = %v, want len = %v", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("JoinPathPts()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
