package space_world_test

import (
	"context"
	"testing"
	"time"

	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_canvas_world "github.com/s4wave/spacewave/sdk/canvas/world"
	s4wave_command "github.com/s4wave/spacewave/sdk/command"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
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

func TestSetSpaceSettingsKeybindingOverridesMergesConcurrentSurfaces(t *testing.T) {
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	initialOverrides := &s4wave_command.KeybindingOverrideSet{Version: 2}
	initialSettings := &space_world.SpaceSettings{
		IndexPath:           "/files",
		PluginIds:           []string{"spacewave-app"},
		KeybindingOverrides: initialOverrides,
	}
	initialOp := space_world_ops.NewSetSpaceSettingsOp("", initialSettings, true, time.Unix(10, 0))
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, initialOp, ""); err != nil {
		t.Fatal(err)
	}

	webOverrides := initialOverrides.CloneVT()
	webOverrides.WebSettings = &s4wave_command.KeybindingOverrideSettings{LeaderCombo: "Ctrl+A"}
	webOp := space_world_ops.NewSetSpaceSettingsOp("", &space_world.SpaceSettings{
		KeybindingOverrides: webOverrides,
	}, true, time.Unix(20, 0))
	webOp.ExpectedKeybindingOverrides = initialOverrides
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, webOp, ""); err != nil {
		t.Fatal(err)
	}

	tuiOverrides := initialOverrides.CloneVT()
	tuiOverrides.TuiSettings = &s4wave_command.KeybindingOverrideSettings{LeaderCombo: "Ctrl+B"}
	tuiOp := space_world_ops.NewSetSpaceSettingsOp("", &space_world.SpaceSettings{
		KeybindingOverrides: tuiOverrides,
	}, true, time.Unix(30, 0))
	tuiOp.ExpectedKeybindingOverrides = initialOverrides
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, tuiOp, ""); err != nil {
		t.Fatal(err)
	}

	got, _, err := space_world.LookupSpaceSettings(ctx, tb.WorldState)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetIndexPath() != "/files" || len(got.GetPluginIds()) != 1 || got.GetPluginIds()[0] != "spacewave-app" {
		t.Fatalf("unrelated settings changed: %#v", got)
	}
	if got.GetKeybindingOverrides().GetWebSettings().GetLeaderCombo() != "Ctrl+A" ||
		got.GetKeybindingOverrides().GetTuiSettings().GetLeaderCombo() != "Ctrl+B" {
		t.Fatalf("surface partitions did not merge: %#v", got.GetKeybindingOverrides())
	}
}

// TestLookupSpaceIndexObjectTypeFollowsDurableObjectMetadata verifies index_path
// resolves the selected root object's type, including through an object subpath.
func TestLookupSpaceIndexObjectTypeFollowsDurableObjectMetadata(t *testing.T) {
	ctx := context.Background()

	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	indexObjects := []struct {
		objectPath string
		indexPath  string
		typeID     string
	}{
		{objectPath: "canvas", indexPath: "canvas/-/nested/path", typeID: s4wave_canvas_world.CanvasTypeID},
		{objectPath: "layout", indexPath: "layout", typeID: s4wave_layout_world.ObjectLayoutTypeID},
	}
	for _, indexObject := range indexObjects {
		if _, err := tb.WorldState.CreateObject(ctx, indexObject.objectPath, nil); err != nil {
			t.Fatalf("CreateObject(%q) failed: %v", indexObject.objectPath, err)
		}
		if err := world_types.SetObjectType(ctx, tb.WorldState, indexObject.objectPath, indexObject.typeID); err != nil {
			t.Fatalf("SetObjectType(%q) failed: %v", indexObject.objectPath, err)
		}
	}

	for i, indexObject := range indexObjects {
		_, _, err := space_world_ops.SetSpaceSettings(
			ctx,
			tb.WorldState,
			"",
			"",
			&space_world.SpaceSettings{IndexPath: indexObject.indexPath},
			true,
			time.Unix(int64(i+1), 0),
		)
		if err != nil {
			t.Fatalf("SetSpaceSettings(%q) failed: %v", indexObject.indexPath, err)
		}

		got, err := space_world.LookupSpaceIndexObjectType(ctx, tb.WorldState)
		if err != nil {
			t.Fatalf("LookupSpaceIndexObjectType(%q) failed: %v", indexObject.indexPath, err)
		}
		if got != indexObject.typeID {
			t.Fatalf("LookupSpaceIndexObjectType(%q) = %q, want %q", indexObject.indexPath, got, indexObject.typeID)
		}
	}
}

// TestLookupSpaceIndexObjectTypeReturnsEmptyWithoutDurableIndexMetadata verifies
// incomplete or stale Space settings do not invent a semantic object type.
func TestLookupSpaceIndexObjectTypeReturnsEmptyWithoutDurableIndexMetadata(t *testing.T) {
	tests := []struct {
		name     string
		settings *space_world.SpaceSettings
	}{
		{name: "missing settings"},
		{name: "empty index path", settings: &space_world.SpaceSettings{}},
		{name: "stale index path", settings: &space_world.SpaceSettings{IndexPath: "/missing"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			tb, err := world_testbed.Default(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tb.Release()

			if test.settings != nil {
				_, _, err := space_world_ops.SetSpaceSettings(
					ctx,
					tb.WorldState,
					"",
					"",
					test.settings,
					true,
					time.Unix(1, 0),
				)
				if err != nil {
					t.Fatalf("SetSpaceSettings failed: %v", err)
				}
			}

			got, err := space_world.LookupSpaceIndexObjectType(ctx, tb.WorldState)
			if err != nil {
				t.Fatalf("LookupSpaceIndexObjectType failed: %v", err)
			}
			if got != "" {
				t.Fatalf("LookupSpaceIndexObjectType = %q, want empty", got)
			}
		})
	}
}
