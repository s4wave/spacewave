package plugin_space_runtime

import (
	"context"
	"slices"
	"testing"
	"time"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
)

// TestGenerationResolvesDeclaredAppPlugins checks forwarding and scheduler exclusion together.
func TestGenerationResolvesDeclaredAppPlugins(t *testing.T) {
	for name, ids := range map[string][]string{
		"spacewave":  {"spacewave-core", "spacewave-web", "spacewave-app", "web"},
		"first-app":  {"first-shell", "first-storage"},
		"second-app": {"second-host", "second-view"},
	} {
		t.Run(name, func(t *testing.T) {
			// Compose a real generation with a distinct parent plugin value.
			tb := newTestbed(t)
			source := newAppPluginSource(ids)
			addTestController(t, tb.Bus, source)
			conf := newTestConfig(tb, "space-a")
			conf.AppPluginIds = append(slices.Clone(ids), "", ids[0])

			// Start the shared runtime with the unnormalized declaration.
			rt, ref, err := StartControllerWithConfig(t.Context(), tb.Bus, conf)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(ref.Release)
			gen := waitGeneration(t, rt, nil)

			// A declaration without duplicates or empty IDs borrows the same runtime.
			canonical := conf.CloneVT()
			canonical.AppPluginIds = canonicalPluginIDs(ids)
			alias, aliasRef, err := StartControllerWithConfig(t.Context(), tb.Bus, canonical)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(aliasRef.Release)
			if alias != rt {
				t.Fatal("equivalent application declarations started different runtimes")
			}

			// Resolve each declared plugin through the runtime's production child bus.
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			for _, id := range ids {
				plugin, inst, loadRef, err := bldr_plugin.ExLoadPlugin(ctx, gen.GetBus(), false, id, nil)
				if err != nil {
					t.Fatal(err)
				}
				if loadRef != nil {
					t.Cleanup(loadRef.Release)
				}
				if plugin != source.plugin {
					t.Fatalf("%s resolved outside its declared parent", id)
				}
				select {
				case got := <-source.loads:
					if got != id {
						t.Fatalf("parent load = %s, want %s", got, id)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}

				// The Space scheduler must decline the same load the parent answered.
				resolvers, err := gen.GetScheduler().HandleDirective(ctx, inst)
				if err != nil {
					t.Fatal(err)
				}
				if len(resolvers) != 0 {
					t.Fatalf("Space scheduler accepted host plugin %s", id)
				}
			}

			// Forwarding and exclusion consume the same canonical declaration.
			if got := gen.GetScheduler().GetPluginStatusCtr().GetValue().GetPlugins(); len(got) != 0 {
				t.Fatalf("host plugins became Space installations: %v", got)
			}
			select {
			case id := <-source.manifests:
				t.Fatalf("host plugin %s fetched a Space manifest", id)
			default:
			}
		})
	}
}

// TestGenerationKeepsUnavailableLocalPluginLocal checks that missing artifacts never borrow a parent load.
func TestGenerationKeepsUnavailableLocalPluginLocal(t *testing.T) {
	// Approve a local plugin whose name belonged to the old fixed host list.
	tb := newTestbed(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	const localID = "spacewave-core"
	if _, _, err := space_world_ops.SetSpaceSettings(ctx, tb.WorldState, "", "", &space_world.SpaceSettings{
		PluginIds: []string{localID},
	}, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	source := newAppPluginSource([]string{"downstream-shell"})
	addTestController(t, tb.Bus, source)

	// Demand the local plugin through the complete runtime while its manifest is absent.
	conf := newTestConfig(tb, "space-a")
	conf.AppPluginIds = source.ids
	rt, ref, err := StartControllerWithConfig(ctx, tb.Bus, conf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ref.Release)
	gen := waitGeneration(t, rt, nil)
	addTestDirective(t, gen.GetBus(), bldr_plugin.NewLoadPluginInstanced(localID, "space-a"))

	// Only the approved manifest lookup reaches the parent, not the plugin load.
	select {
	case id := <-source.manifests:
		if id != localID {
			t.Fatalf("manifest lookup = %s, want %s", id, localID)
		}
	case id := <-source.loads:
		t.Fatalf("local plugin load reached parent: %s", id)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	snapshot, err := gen.GetScheduler().GetPluginStatusCtr().WaitValueWithValidator(ctx, func(snapshot *bldr_plugin.PluginStatusSnapshot) (bool, error) {
		return slices.ContainsFunc(snapshot.GetPlugins(), func(status *bldr_plugin.PluginStatus) bool {
			return status.GetPluginId() == localID && status.GetState() == bldr_plugin.PluginState_PluginState_REQUESTED
		}), nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.GetPlugins()) != 1 {
		t.Fatalf("Space plugins = %v, want only the local plugin", snapshot.GetPlugins())
	}

	// Keep the unresolved local demand live long enough to detect an implicit fallback.
	select {
	case id := <-source.loads:
		t.Fatalf("unavailable local plugin load reached parent: %s", id)
	case <-time.After(50 * time.Millisecond):
	}
}
