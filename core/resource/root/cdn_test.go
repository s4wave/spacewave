package resource_root

import (
	"testing"

	"github.com/s4wave/spacewave/core/cdn"
)

func TestOrdinarySpaceLookupDoesNotMountCdn(t *testing.T) {
	// Exercise the CDN hook used by every session SharedObject mount.
	server := newCloseTestServer(t)
	defer server.Close()
	for _, spaceID := range []string{"", "01ordinaryspace000000000000"} {
		so, meta := server.lookupCdnSharedObject(spaceID)
		if so != nil || meta != nil {
			t.Fatalf("ordinary Space %q resolved as CDN", spaceID)
		}
	}

	// An unrelated mount must not create a CDN refresh subscriber.
	if server.cdnRegistry.NotifyRootChanged(cdn.SpaceID()) {
		t.Fatal("ordinary Space lookup initialized the public CDN")
	}
}
