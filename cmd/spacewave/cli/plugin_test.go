//go:build !js

package spacewave_cli

import (
	"flag"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

func TestPluginSubcommandsUseClientFlags(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cli.Command
	}{
		{"list", buildPluginListCommand()},
		{"add", buildPluginSetCommand(true)},
		{"remove", buildPluginSetCommand(false)},
		{"background", buildPluginBackgroundCommand()},
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
	// Require the object-key default and the target-db flag.
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
				ManifestRef: "manifest-ref",
				ManifestRev: 7,
			}
			want := "plugin-id  " + tt.label + "  rev 7 selected, none running  " + detail + "  description\n"
			if got := formatPluginStatus(status, time.UTC); got != want {
				t.Fatalf("formatted row = %q, want %q", got, want)
			}
		})
	}
}

func TestMarshalPluginListJSONIncludesLifecycleFields(t *testing.T) {
	data, err := marshalPluginListJSON([]*s4wave_space.SpacePluginStatus{{
		PluginId:    "plugin-id",
		State:       s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_LOADING,
		Detail:      "connected-awaiting-registration",
		ManifestRef: "manifest-ref",
		ManifestRev: 42,
	}})
	if err != nil {
		t.Fatalf("marshal plugin list: %v", err)
	}
	got := string(data)
	for _, field := range []string{`"state":"SpacePluginLifecycleState_LOADING"`, `"detail":"connected-awaiting-registration"`, `"manifestRef":"manifest-ref"`, `"manifestRev":`} {
		if !strings.Contains(got, field) {
			t.Fatalf("JSON = %s, missing %s", got, field)
		}
	}
}

// TestFormatPluginSwapStates checks the row plugin list prints for each swap
// state between the selected and running manifests.
func TestFormatPluginSwapStates(t *testing.T) {
	at := func(clock string) *timestamppb.Timestamp {
		ts, err := time.Parse(time.TimeOnly, clock)
		if err != nil {
			t.Fatal(err)
		}
		return timestamppb.New(ts)
	}
	loaded := s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_LOADED
	tests := []struct {
		name   string
		status *s4wave_space.SpacePluginStatus
		want   string
	}{{
		name: "live",
		status: &s4wave_space.SpacePluginStatus{
			PluginId: "colors", State: loaded,
			ManifestRef: "ref-5", ManifestRev: 5, RunningManifestRef: "ref-5", RunningManifestRev: 5,
			Swap: &bldr_plugin.PluginSwap{State: &bldr_plugin.PluginSwap_Settled{Settled: &bldr_plugin.PluginSwapSettled{AdmittedAt: at("21:40:00")}}},
		},
		want: "colors  loaded  rev 5 live\n",
	}, {
		name: "preparing",
		status: &s4wave_space.SpacePluginStatus{
			PluginId: "colors", State: loaded,
			ManifestRef: "ref-6", ManifestRev: 6, RunningManifestRef: "ref-5", RunningManifestRev: 5,
			Swap: &bldr_plugin.PluginSwap{State: &bldr_plugin.PluginSwap_Preparing{Preparing: &bldr_plugin.PluginSwapPreparing{
				CandidateManifestRef: "ref-6", CandidateManifestRev: 6, StartedAt: at("21:41:00"),
			}}},
		},
		want: "colors  loaded  rev 6 selected, rev 5 running; preparing rev 6 since 21:41:00\n",
	}, {
		name: "waiting",
		status: &s4wave_space.SpacePluginStatus{
			PluginId: "colors", State: loaded,
			ManifestRef: "ref-6", ManifestRev: 6, RunningManifestRef: "ref-5", RunningManifestRev: 5,
			Swap: &bldr_plugin.PluginSwap{State: &bldr_plugin.PluginSwap_Waiting{Waiting: &bldr_plugin.PluginSwapWaiting{
				CandidateManifestRef: "ref-6", CandidateManifestRev: 6, SinceAt: at("21:41:01"),
			}}},
		},
		want: "colors  loaded  rev 6 selected, rev 5 running; rev 6 waits since 21:41:01 for the running worker" +
			" to answer its activation check; it swaps when the worker answers or exits" +
			" (a browser worker exits when its page reloads); if it does neither, run" +
			" spacewave plugin restart colors\n",
	}, {
		name: "failed",
		status: &s4wave_space.SpacePluginStatus{
			PluginId: "colors", State: loaded,
			ManifestRef: "ref-6", ManifestRev: 6, RunningManifestRef: "ref-5", RunningManifestRev: 5,
			Swap: &bldr_plugin.PluginSwap{State: &bldr_plugin.PluginSwap_Failed{Failed: &bldr_plugin.PluginSwapFailed{
				CandidateManifestRef: "ref-6", CandidateManifestRev: 6,
				ErrorMessage: "prepare plugin: startup failed", FailedAt: at("21:42:00"), RetryAt: at("21:42:30"),
			}}},
		},
		want: "colors  loaded  rev 6 selected, rev 5 running; rev 6 failed at 21:42:00: prepare plugin: startup failed; retry at 21:42:30\n",
	}, {
		name: "failed without retry",
		status: &s4wave_space.SpacePluginStatus{
			PluginId: "colors", State: s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_FAILED,
			ManifestRef: "ref-6", ManifestRev: 6,
			Swap: &bldr_plugin.PluginSwap{State: &bldr_plugin.PluginSwap_Failed{Failed: &bldr_plugin.PluginSwapFailed{
				CandidateManifestRef: "ref-6", CandidateManifestRev: 6,
				ErrorMessage: "prepare plugin: installed plugin has no compatible host", FailedAt: at("21:43:00"),
			}}},
		},
		want: "colors  failed  rev 6 selected, none running; rev 6 failed at 21:43:00: prepare plugin:" +
			" installed plugin has no compatible host; retries on the next deploy or host change\n",
	}, {
		name: "same rev, other build",
		status: &s4wave_space.SpacePluginStatus{
			PluginId: "colors", State: loaded,
			ManifestRef: "ref-6b", ManifestRev: 6, RunningManifestRef: "ref-6a", RunningManifestRev: 6,
		},
		want: "colors  loaded  rev 6 selected, another build of rev 6 running\n",
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatPluginStatus(tt.status, time.UTC)
			t.Logf("%s: %s", tt.name, strings.TrimSuffix(got, "\n"))
			if got != tt.want {
				t.Fatalf("formatted row = %q, want %q", got, tt.want)
			}
		})
	}
}
