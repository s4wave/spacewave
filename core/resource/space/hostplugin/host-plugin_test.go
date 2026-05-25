package hostplugin

import (
	"context"
	"testing"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

func TestResolveUsesStoredResourceOwner(t *testing.T) {
	ctx := bldr_plugin.WithPluginContextInfo(
		context.Background(),
		bldr_plugin.NewPluginContextInfo(
			bldr_plugin.NewPluginMeta("spacewave", "context-plugin", "desktop/darwin/arm64", "dev"),
		),
	)

	if got := Resolve(ctx, "spacewave-core"); got != "spacewave-core" {
		t.Fatalf("expected stored host plugin id, got %q", got)
	}
}

func TestResolveFallsBackToContext(t *testing.T) {
	ctx := bldr_plugin.WithPluginContextInfo(
		context.Background(),
		bldr_plugin.NewPluginContextInfo(
			bldr_plugin.NewPluginMeta("spacewave", "context-plugin", "desktop/darwin/arm64", "dev"),
		),
	)

	if got := Resolve(ctx, ""); got != "context-plugin" {
		t.Fatalf("expected context host plugin id, got %q", got)
	}
}
