package plugin_host_scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/sirupsen/logrus"
)

// blockingUpdateGuard represents a plugin that still owns active work.
type blockingUpdateGuard struct {
	entered chan struct{}
	allow   chan struct{}
}

// Prepare waits for the test's explicit release of the current generation.
func (g *blockingUpdateGuard) Prepare(ctx context.Context, _ *plugin.PrepareUpdateRequest) (*plugin.PrepareUpdateResponse, error) {
	close(g.entered)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-g.allow:
		return &plugin.PrepareUpdateResponse{}, nil
	}
}

// TestGuardedPluginReplacement keeps the running generation until its RPC guard
// permits replacement, and preserves it when the request is canceled.
func TestGuardedPluginReplacement(t *testing.T) {
	for _, cancelUpdate := range []bool{false, true} {
		t.Run(map[bool]string{false: "approved", true: "canceled"}[cancelUpdate], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			c := &Controller{conf: &Config{UpdateGuardPluginIds: []string{"core"}}, le: logrus.NewEntry(logrus.New())}
			_, instance := c.newPluginInstance(pluginReference{pluginID: "core"})
			old := &executePluginArgs{pluginHost: &testPluginHost{id: "old"}}
			next := &executePluginArgs{pluginHost: &testPluginHost{id: "new"}}
			instance.setExecutePluginState(old)
			guard := &blockingUpdateGuard{entered: make(chan struct{}), allow: make(chan struct{})}
			mux := srpc.NewMux()
			if err := plugin.SRPCRegisterUpdateGuard(mux, guard); err != nil {
				t.Fatal(err)
			}
			instance.runningPluginCtr.SetValue(plugin.NewRunningPlugin(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))))
			done := make(chan error, 1)
			go func() { done <- instance.execGuardedPluginUpdate(ctx, next) }()
			select {
			case <-guard.entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if instance.executePluginRoutine.GetState() != old {
				t.Fatal("replaced a busy plugin")
			}
			if cancelUpdate {
				cancel()
			} else {
				close(guard.allow)
			}
			if err := <-done; (err != nil) != cancelUpdate {
				t.Fatalf("update returned %v", err)
			}
			want := next
			if cancelUpdate {
				want = old
			}
			if instance.executePluginRoutine.GetState() != want {
				t.Fatal("incorrect generation after guard returned")
			}
		})
	}
}

// TestPluginCopyGapKeepsNewerGeneration prevents an older durable fallback from
// replacing a newer embedded generation while the remote copy is pending.
func TestPluginCopyGapKeepsNewerGeneration(t *testing.T) {
	c := &Controller{conf: &Config{}, le: logrus.NewEntry(logrus.New())}
	_, instance := c.newPluginInstance(pluginReference{pluginID: "app"})
	current := &executePluginArgs{manifestSnapshot: &manifest.ManifestSnapshot{
		ManifestRef: newTestManifestRef("app", "desktop/linux/amd64", 12, "embedded").GetManifestRef(),
		Manifest:    &manifest.Manifest{Meta: manifest.NewManifestMeta("app", manifest.BuildType_RELEASE, "desktop/linux/amd64", 12)},
	}}
	older := &executePluginArgs{manifestSnapshot: &manifest.ManifestSnapshot{
		ManifestRef: newTestManifestRef("app", "desktop/linux/amd64", 10, "local").GetManifestRef(),
		Manifest:    &manifest.Manifest{Meta: manifest.NewManifestMeta("app", manifest.BuildType_RELEASE, "desktop/linux/amd64", 10)},
	}}
	instance.setExecutePluginState(current)
	if instance.setExecutePluginState(older) || instance.executePluginRoutine.GetState() != current {
		t.Fatal("copy gap replaced the newer running generation")
	}
	newer := &executePluginArgs{manifestSnapshot: &manifest.ManifestSnapshot{
		ManifestRef: newTestManifestRef("app", "desktop/linux/amd64", 13, "local").GetManifestRef(),
		Manifest:    &manifest.Manifest{Meta: manifest.NewManifestMeta("app", manifest.BuildType_RELEASE, "desktop/linux/amd64", 13)},
	}}
	if !instance.setExecutePluginState(newer) || instance.executePluginRoutine.GetState() != newer {
		t.Fatal("ready newer generation did not replace the current plugin")
	}
}
