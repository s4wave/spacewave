package statusprojector

import (
	"testing"

	"github.com/s4wave/spacewave/core/cdn"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
)

func TestBuildSpaceProjectionBoundsAndRoutesToApp(t *testing.T) {
	rows := make([]*spaceProjectionRow, 0, maxProjectedSpaces+1)
	for i := 1; i <= maxProjectedSpaces+1; i++ {
		rows = append(rows, &spaceProjectionRow{
			sessionIndex: 3,
			sessionLabel: "Cloud",
			space:        testSpaceEntry("space-"+string(rune('a'+i-1)), "Space "+string(rune('A'+i-1)), "shared"),
		})
	}

	projection := buildSpaceProjection(rows)
	if len(projection) != maxProjectedSpaces {
		t.Fatalf("space rows = %d, want %d", len(projection), maxProjectedSpaces)
	}
	first := projection[0]
	if first.GetRoute() != "/u/3/so/space-a" {
		t.Fatalf("first route = %q, want app handoff route", first.GetRoute())
	}
	if first.GetDetail() != "Cloud - Shared" {
		t.Fatalf("first detail = %q, want session plus source", first.GetDetail())
	}
	if first.GetStatusText() != "Shared" {
		t.Fatalf("first status = %q, want source label", first.GetStatusText())
	}
}

func TestBuildSessionSpacesFiltersCdnSpace(t *testing.T) {
	spaces, err := buildSessionSpaces(&sobject.SharedObjectList{
		SharedObjects: []*sobject.SharedObjectListEntry{
			testSpaceEntry("regular", "Regular", "created").GetEntry(),
			testSpaceEntry(cdn.SpaceID(), "CDN", "created").GetEntry(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(spaces) != 1 {
		t.Fatalf("spaces = %d, want only regular space", len(spaces))
	}
	if spaceID(spaces[0]) != "regular" {
		t.Fatalf("space id = %q, want regular", spaceID(spaces[0]))
	}
}

func testSpaceEntry(id, name, source string) *space.SpaceSoListEntry {
	metaBytes, err := (&space.SpaceSoMeta{Name: name}).MarshalVT()
	if err != nil {
		panic(err)
	}
	return &space.SpaceSoListEntry{
		Entry: &sobject.SharedObjectListEntry{
			Ref: &sobject.SharedObjectRef{
				ProviderResourceRef: &provider.ProviderResourceRef{
					Id: id,
				},
				BlockStoreId: id,
			},
			Meta: &sobject.SharedObjectMeta{
				BodyType: space.SpaceBodyType,
				BodyMeta: metaBytes,
			},
			Source: source,
		},
		SpaceMeta: &space.SpaceSoMeta{Name: name},
	}
}
