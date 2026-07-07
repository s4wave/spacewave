package resource_space

import (
	"testing"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

func TestAvailablePluginsFromCatalogKeepsHighestRev(t *testing.T) {
	catalog := map[string]*bldr_manifest.ManifestMeta{}
	// Two revisions of the same plugin plus a distinct plugin; the catalog must
	// keep the highest revision per manifest ID.
	for _, meta := range []*bldr_manifest.ManifestMeta{
		{ManifestId: "spacewave-notes", Rev: 2, Description: "notes v2"},
		{ManifestId: "spacewave-notes", Rev: 5, Description: "notes v5"},
		{ManifestId: "spacewave-notes", Rev: 1, Description: "notes v1"},
		{ManifestId: "spacewave-v86", Rev: 3, Description: "vm"},
		{ManifestId: "", Rev: 9, Description: "unnamed"},
	} {
		addManifestToCatalog(catalog, meta)
	}

	got := availablePluginsFromCatalog(catalog)
	if len(got) != 2 {
		t.Fatalf("expected 2 plugins, got %d: %+v", len(got), got)
	}

	// Sorted by plugin ID: notes before v86.
	if got[0].GetPluginId() != "spacewave-notes" || got[1].GetPluginId() != "spacewave-v86" {
		t.Fatalf("unexpected sort order: %s, %s", got[0].GetPluginId(), got[1].GetPluginId())
	}
	if got[0].GetRevision() != "5" {
		t.Fatalf("expected highest revision 5, got %q", got[0].GetRevision())
	}
	if got[0].GetDescription() != "notes v5" {
		t.Fatalf("expected highest-rev description, got %q", got[0].GetDescription())
	}
	if got[1].GetRevision() != "3" {
		t.Fatalf("expected revision 3 for v86, got %q", got[1].GetRevision())
	}
}

func TestAvailablePluginsFromCatalogEmpty(t *testing.T) {
	got := availablePluginsFromCatalog(map[string]*bldr_manifest.ManifestMeta{})
	if len(got) != 0 {
		t.Fatalf("expected empty catalog, got %+v", got)
	}
}
