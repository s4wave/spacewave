package web_pkg

import (
	"reflect"
	"testing"
)

func TestWebPkgRefSliceAppendWebPkgRootMergesWithImports(t *testing.T) {
	// Start with an empty package reference slice.
	var refs WebPkgRefSlice
	var dirty bool

	// Add the web package root to the reference slice.
	refs, dirty = refs.AppendWebPkgRoot("@s4wave/web", "/repo/web")

	// Verify the new package reference contains its root without imports.
	if !dirty {
		t.Fatal("AppendWebPkgRoot returned dirty=false")
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if refs[0].GetWebPkgRoot() != "/repo/web" {
		t.Fatalf("expected root /repo/web, got %q", refs[0].GetWebPkgRoot())
	}
	if len(refs[0].GetImports()) != 0 {
		t.Fatalf("expected no imports, got %v", refs[0].GetImports())
	}

	// Merge the state import into the existing web package reference.
	refs, dirty = refs.AppendWebPkgRef("@s4wave/web", "/repo/web", "state/index.tsx")

	// Verify the state import was added without duplicating the package reference.
	if !dirty {
		t.Fatal("AppendWebPkgRef returned dirty=false")
	}
	if len(refs) != 1 {
		t.Fatalf("expected merged ref, got %d refs", len(refs))
	}
	if got := refs[0].GetImports(); !reflect.DeepEqual(got, []string{"state/index.tsx"}) {
		t.Fatalf("expected merged imports, got %v", got)
	}
}

func TestWebPkgRefSliceAppendWebPkgRefValueMergesFields(t *testing.T) {
	// Create a package reference with an import and a cross reference.
	refs := WebPkgRefSlice{{
		WebPkgId:   "@s4wave/web",
		WebPkgRoot: "/repo/web",
		Imports:    []string{"object/object.ts"},
		CrossRefs:  []string{"react"},
	}}

	// Merge an incoming package reference with overlapping imports and cross references.
	var dirty bool
	refs, dirty = refs.AppendWebPkgRefValue(&WebPkgRef{
		WebPkgId:   "@s4wave/web",
		WebPkgRoot: "/repo/web",
		Imports:    []string{"state/index.tsx", "object/object.ts"},
		CrossRefs:  []string{"react", "sonner"},
	})

	// Verify merged package fields are distinct and sorted.
	if !dirty {
		t.Fatal("AppendWebPkgRefValue returned dirty=false")
	}
	if len(refs) != 1 {
		t.Fatalf("expected merged ref, got %d refs", len(refs))
	}
	if got := refs[0].GetImports(); !reflect.DeepEqual(got, []string{"object/object.ts", "state/index.tsx"}) {
		t.Fatalf("expected merged imports, got %v", got)
	}
	if got := refs[0].GetCrossRefs(); !reflect.DeepEqual(got, []string{"react", "sonner"}) {
		t.Fatalf("expected merged cross refs, got %v", got)
	}
}
