//go:build !js

package spacewave_cli

import (
	"flag"
	"io"
	"strings"
	"testing"

	"github.com/aperturerobotics/cli"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

func TestPluginSubcommandsUseClientFlags(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cli.Command
	}{
		{"list", buildPluginListCommand()},
		{"add", buildPluginAddCommand()},
		{"remove", buildPluginRemoveCommand()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := flag.NewFlagSet(tt.name, flag.ContinueOnError)
			set.SetOutput(io.Discard)
			for _, fl := range tt.cmd.Flags {
				if err := fl.Apply(set); err != nil {
					t.Fatalf("apply flag: %v", err)
				}
			}
			for _, name := range []string{"state-path", "socket-path", "session-index", "space"} {
				if set.Lookup(name) == nil {
					t.Fatalf("%s flag missing", name)
				}
			}
		})
	}
}

func TestPluginImportManifestObjectKeyFlag(t *testing.T) {
	cmd := buildPluginImportManifestCommand(nil)
	set := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	for _, fl := range cmd.Flags {
		if err := fl.Apply(set); err != nil {
			t.Fatalf("apply flag: %v", err)
		}
	}
	objectKey := set.Lookup("object-key")
	if objectKey == nil {
		t.Fatal("object-key flag missing")
	}
	if objectKey.DefValue != defaultPluginHostObjectKey {
		t.Fatalf("object-key default = %q, want %q", objectKey.DefValue, defaultPluginHostObjectKey)
	}
	if set.Lookup("target-db") == nil {
		t.Fatal("target-db flag missing")
	}
}

func TestFormatPluginStatusPreservesLifecycleState(t *testing.T) {
	tests := []struct {
		name  string
		state s4wave_space.SpacePluginLifecycleState
		label string
	}{
		{
			name:  "unknown",
			state: s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_UNKNOWN,
			label: "unknown",
		},
		{
			name:  "configured",
			state: s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_CONFIGURED,
			label: "configured",
		},
		{
			name:  "loading",
			state: s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_LOADING,
			label: "loading",
		},
		{
			name:  "loaded",
			state: s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_LOADED,
			label: "loaded",
		},
		{
			name:  "failed",
			state: s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_FAILED,
			label: "failed",
		},
		{
			name:  "retrying",
			state: s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_RETRYING,
			label: "retrying",
		},
		{
			name:  "removed",
			state: s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_REMOVED,
			label: "removed",
		},
		{
			name:  "upgraded",
			state: s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_UPGRADED,
			label: "upgraded",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detail := "state detail"
			status := &s4wave_space.SpacePluginStatus{
				PluginId:    "plugin-id",
				Loaded:      true,
				State:       tt.state,
				Detail:      detail,
				Description: "description",
			}
			want := "plugin-id  " + tt.label + "  " + detail + "  description\n"
			if got := formatPluginStatus(status); got != want {
				t.Fatalf("formatted row = %q, want %q", got, want)
			}
		})
	}
}

func TestMarshalPluginListJSONIncludesLifecycleFields(t *testing.T) {
	data, err := marshalPluginListJSON([]*s4wave_space.SpacePluginStatus{{
		PluginId: "plugin-id",
		State:    s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_LOADING,
		Detail:   "connected-awaiting-registration",
	}})
	if err != nil {
		t.Fatalf("marshal plugin list: %v", err)
	}
	got := string(data)
	for _, field := range []string{`"state":"SpacePluginLifecycleState_LOADING"`, `"detail":"connected-awaiting-registration"`} {
		if !strings.Contains(got, field) {
			t.Fatalf("JSON = %s, missing %s", got, field)
		}
	}
}
