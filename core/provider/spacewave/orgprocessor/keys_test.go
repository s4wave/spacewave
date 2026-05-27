package orgprocessor

import (
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
)

func TestKeysFollowsOrgListCache(t *testing.T) {
	list := &sobject.SharedObjectList{
		SharedObjects: []*sobject.SharedObjectListEntry{
			orgProcessorListEntry("org-owner", s4wave_org.OrgBodyType),
			orgProcessorListEntry("org-member", s4wave_org.OrgBodyType),
			orgProcessorListEntry("space-1", "space"),
		},
	}

	if got := Keys(list, nil, false); len(got) != 0 {
		t.Fatalf("org processor keys before org cache = %v, want empty", got)
	}

	got := Keys(list, []*api.OrgResponse{
		{Id: "org-owner", Role: "org:owner"},
		{Id: "org-member", Role: "org:member"},
	}, true)
	if len(got) != 1 || got[0] != "org-owner" {
		t.Fatalf("org processor keys after org cache = %v, want [org-owner]", got)
	}
}

func orgProcessorListEntry(id string, bodyType string) *sobject.SharedObjectListEntry {
	return &sobject.SharedObjectListEntry{
		Ref:  sobject.NewSharedObjectRef("spacewave", "acct-1", id, "block-"+id),
		Meta: &sobject.SharedObjectMeta{BodyType: bodyType},
	}
}
