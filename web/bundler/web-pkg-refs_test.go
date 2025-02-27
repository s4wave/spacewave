package bldr_web_bundler

import (
	"reflect"
	"testing"
)

func TestCompactWebPkgRefConfigs(t *testing.T) {
	tests := []struct {
		name string
		refs []*WebPkgRefConfig
		want []*WebPkgRefConfig
	}{
		{
			name: "empty slice",
			refs: []*WebPkgRefConfig{},
			want: []*WebPkgRefConfig{},
		},
		{
			name: "single element",
			refs: []*WebPkgRefConfig{
				{Id: "pkg1", Imports: []string{"a", "b"}},
			},
			want: []*WebPkgRefConfig{
				{Id: "pkg1", Imports: []string{"a", "b"}},
			},
		},
		{
			name: "no duplicates",
			refs: []*WebPkgRefConfig{
				{Id: "pkg1", Imports: []string{"a", "b"}},
				{Id: "pkg2", Imports: []string{"c", "d"}},
			},
			want: []*WebPkgRefConfig{
				{Id: "pkg1", Imports: []string{"a", "b"}},
				{Id: "pkg2", Imports: []string{"c", "d"}},
			},
		},
		{
			name: "merge duplicates",
			refs: []*WebPkgRefConfig{
				{Id: "pkg1", Imports: []string{"a", "b"}},
				{Id: "pkg1", Imports: []string{"c", "d"}},
			},
			want: []*WebPkgRefConfig{
				{Id: "pkg1", Imports: []string{"a", "b", "c", "d"}},
			},
		},
		{
			name: "merge exclude flag",
			refs: []*WebPkgRefConfig{
				{Id: "pkg1", Imports: []string{"a", "b"}},
				{Id: "pkg1", Exclude: true},
			},
			want: []*WebPkgRefConfig{
				{Id: "pkg1", Exclude: true, Imports: []string{"a", "b"}},
			},
		},
		{
			name: "skip empty ids",
			refs: []*WebPkgRefConfig{
				{Id: "", Imports: []string{"a", "b"}},
				{Id: "pkg1", Imports: []string{"c", "d"}},
				{Id: "", Imports: []string{"e", "f"}},
			},
			want: []*WebPkgRefConfig{
				{Id: "pkg1", Imports: []string{"c", "d"}},
			},
		},
		{
			name: "complex case with multiple duplicates",
			refs: []*WebPkgRefConfig{
				{Id: "pkg2", Imports: []string{"a", "b"}},
				{Id: "pkg1", Imports: []string{"c", "d"}},
				{Id: "pkg3", Imports: []string{"e", "f"}},
				{Id: "pkg1", Imports: []string{"g", "h"}},
				{Id: "pkg2", Exclude: true},
				{Id: "pkg3", Imports: []string{"i", "j"}},
			},
			want: []*WebPkgRefConfig{
				{Id: "pkg1", Imports: []string{"c", "d", "g", "h"}},
				{Id: "pkg2", Exclude: true, Imports: []string{"a", "b"}},
				{Id: "pkg3", Imports: []string{"e", "f", "i", "j"}},
			},
		},
		{
			name: "deduplicate imports",
			refs: []*WebPkgRefConfig{
				{Id: "pkg1", Imports: []string{"a", "b", "c"}},
				{Id: "pkg1", Imports: []string{"b", "c", "d"}},
			},
			want: []*WebPkgRefConfig{
				{Id: "pkg1", Imports: []string{"a", "b", "c", "d"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy of the input to avoid modifying the test case
			input := make([]*WebPkgRefConfig, len(tt.refs))
			for i, ref := range tt.refs {
				input[i] = &WebPkgRefConfig{
					Id:      ref.Id,
					Exclude: ref.Exclude,
					Imports: append([]string{}, ref.Imports...),
				}
			}

			got := CompactWebPkgRefConfigs(input)

			// Check if the result has the expected length
			if len(got) != len(tt.want) {
				t.Errorf("CompactWebPkgRefConfigs() got %d elements, want %d", len(got), len(tt.want))
				return
			}

			// Check each element
			for i, wantRef := range tt.want {
				if i >= len(got) {
					t.Errorf("CompactWebPkgRefConfigs() missing expected element at index %d", i)
					continue
				}

				gotRef := got[i]
				if gotRef.GetId() != wantRef.GetId() {
					t.Errorf("Element %d: Id = %q, want %q", i, gotRef.GetId(), wantRef.GetId())
				}
				if gotRef.GetExclude() != wantRef.GetExclude() {
					t.Errorf("Element %d: Exclude = %v, want %v", i, gotRef.GetExclude(), wantRef.GetExclude())
				}
				if !reflect.DeepEqual(gotRef.GetImports(), wantRef.GetImports()) {
					t.Errorf("Element %d: Imports = %v, want %v", i, gotRef.GetImports(), wantRef.GetImports())
				}
			}
		})
	}
}
