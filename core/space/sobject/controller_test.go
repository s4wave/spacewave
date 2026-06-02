package space_sobject

import (
	"testing"

	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
)

func TestNewSpaceWorldEngineConfigDisablesChangelog(t *testing.T) {
	sharedObjectRef := &sobject.SharedObjectRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "test-space",
			ProviderId:        "local",
			ProviderAccountId: "test-account",
		},
	}

	conf := newSpaceWorldEngineConfig(sharedObjectRef, &Config{})
	if conf.GetEngineId() != space.SpaceEngineId(sharedObjectRef) {
		t.Fatalf("unexpected engine id: %q", conf.GetEngineId())
	}
	if !conf.GetInitWorldOp().GetLastChangeDisable() {
		t.Fatal("expected Space world init to disable changelog")
	}
}
