package space_world_test

import (
	"context"
	"testing"
	"time"

	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_command "github.com/s4wave/spacewave/sdk/command"
)

// TestLookupSpaceSettingsMissing checks missing settings return nil without error.
func TestLookupSpaceSettingsMissing(t *testing.T) {
	ctx := context.Background()

	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	settings, state, err := space_world.LookupSpaceSettings(ctx, tb.WorldState)
	if err != nil {
		t.Fatal(err)
	}
	if settings != nil {
		t.Fatalf("expected nil settings, got %#v", settings)
	}
	if state != nil {
		t.Fatalf("expected nil state, got %#v", state)
	}
}

// TestSetSpaceSettingsKeybindingOverridesPreservesSettingsFields verifies the
// settings world op stores keybinding overrides alongside the existing index
// and plugin settings supplied by higher-level helpers.
func TestSetSpaceSettingsKeybindingOverridesPreservesSettingsFields(t *testing.T) {
	ctx := context.Background()

	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	initialOp := space_world_ops.NewSetSpaceSettingsOp(
		"",
		&space_world.SpaceSettings{
			IndexPath: "/files",
			PluginIds: []string{
				"spacewave-app",
				"spacewave-terminal",
			},
		},
		true,
		time.Unix(10, 0),
	)
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, initialOp, ""); err != nil {
		t.Fatalf("ApplyWorldOp initial settings failed: %v", err)
	}

	overrideOp := space_world_ops.NewSetSpaceSettingsOp(
		"",
		&space_world.SpaceSettings{
			IndexPath: "/files",
			PluginIds: []string{
				"spacewave-app",
				"spacewave-terminal",
			},
			KeybindingOverrides: &s4wave_command.KeybindingOverrideSet{
				Version: 1,
				Overrides: []*s4wave_command.KeybindingCommandOverride{{
					CommandId:         "spacewave.palette",
					ClearedBindingIds: []string{"palette-default"},
					Bindings: []*s4wave_command.CommandBinding{{
						Id: "palette-space",
						Binding: &s4wave_command.CommandBinding_Combo{
							Combo: &s4wave_command.KeyCombo{Combo: "Ctrl+K"},
						},
						When: s4wave_command.CommandFocusContext_COMMAND_FOCUS_CONTEXT_GLOBAL,
					}},
				}},
			},
		},
		true,
		time.Unix(20, 0),
	)
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, overrideOp, ""); err != nil {
		t.Fatalf("ApplyWorldOp keybinding settings failed: %v", err)
	}

	settings, _, err := space_world.LookupSpaceSettings(ctx, tb.WorldState)
	if err != nil {
		t.Fatal(err)
	}
	if settings == nil {
		t.Fatal("expected settings object after SetSpaceSettingsOp")
	}
	if settings.GetIndexPath() != "/files" {
		t.Fatalf("index_path = %q", settings.GetIndexPath())
	}
	if got := settings.GetPluginIds(); len(got) != 2 || got[0] != "spacewave-app" || got[1] != "spacewave-terminal" {
		t.Fatalf("plugin_ids = %#v", got)
	}
	overrides := settings.GetKeybindingOverrides().GetOverrides()
	if len(overrides) != 1 {
		t.Fatalf("expected one keybinding override, got %d", len(overrides))
	}
	if overrides[0].GetCommandId() != "spacewave.palette" {
		t.Fatalf("override command_id = %q", overrides[0].GetCommandId())
	}
	if got := overrides[0].GetClearedBindingIds(); len(got) != 1 || got[0] != "palette-default" {
		t.Fatalf("cleared_binding_ids = %#v", got)
	}
	if bindings := overrides[0].GetBindings(); len(bindings) != 1 || bindings[0].GetId() != "palette-space" {
		t.Fatalf("bindings = %#v", bindings)
	}
}
