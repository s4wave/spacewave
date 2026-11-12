//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/net/hash"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

func TestSpaceDeployCommandAdvertisesManifestSetDefault(t *testing.T) {
	// Require the deploy command to advertise a manifest set.
	statePath := ""
	sessionIdx := uint(1)
	cmd := newSpaceDeployCommand(&statePath, &sessionIdx)
	if !strings.Contains(cmd.Usage, "manifest set") {
		t.Fatalf("usage = %q", cmd.Usage)
	}
	found := false
	for _, flag := range cmd.Flags {
		if strings.Contains(flag.String(), "plugin-host") {
			found = true
		}
	}
	if !found {
		t.Fatal("object-key flag does not advertise plugin-host default")
	}
}

// deployFixture builds a deployed set of one manifest and the listing ref
// strings of it and of an older build.
func deployFixture(t *testing.T) (set *deployedSet, deployed, older string) {
	t.Helper()
	objRef := func(data string) *bucket.ObjectRef {
		h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte(data))
		if err != nil {
			t.Fatal(err)
		}
		return &bucket.ObjectRef{BucketId: "space", RootRef: &block.BlockRef{Hash: h}}
	}
	ref := objRef("rev 2")
	set = &deployedSet{manifestID: "colors", refs: []*bldr_manifest.ManifestRef{{
		Meta:        bldr_manifest.NewManifestMeta("colors", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 2),
		ManifestRef: &bucket.ObjectRef{BucketId: "devtool", RootRef: ref.GetRootRef()},
	}}}
	return set, ref.MarshalB58(), objRef("rev 1").MarshalB58()
}

// deployClock parses a clock time on the zero day in UTC.
func deployClock(t *testing.T, clock string) *timestamppb.Timestamp {
	t.Helper()
	ts, err := time.Parse(time.TimeOnly, clock)
	if err != nil {
		t.Fatal(err)
	}
	return timestamppb.New(ts)
}

// deployStates returns a recv over states that then fails with err.
func deployStates(err error, plugins ...*s4wave_space.SpacePluginStatus) func() (*s4wave_space.SpaceContentsState, error) {
	return func() (*s4wave_space.SpaceContentsState, error) {
		if len(plugins) == 0 {
			return nil, err
		}
		p := plugins[0]
		plugins = plugins[1:]
		return &s4wave_space.SpaceContentsState{Plugins: []*s4wave_space.SpacePluginStatus{p}}, nil
	}
}

func TestFollowDeployWalksSwapStates(t *testing.T) {
	// Walk the swap states the scheduler publishes for a deploy that fails
	// once, retries, waits on the running worker and goes live.
	set, deployed, older := deployFixture(t)
	row := func(selected string, swap *bldr_plugin.PluginSwap, running string, runningRev uint64) *s4wave_space.SpacePluginStatus {
		return &s4wave_space.SpacePluginStatus{
			PluginId: "colors", ManifestRef: selected, ManifestRev: 2,
			RunningManifestRef: running, RunningManifestRev: runningRev, Swap: swap,
		}
	}
	preparing := &bldr_plugin.PluginSwap{State: &bldr_plugin.PluginSwap_Preparing{Preparing: &bldr_plugin.PluginSwapPreparing{
		CandidateManifestRef: deployed, CandidateManifestRev: 2, StartedAt: deployClock(t, "21:41:00"),
	}}}
	failed := &bldr_plugin.PluginSwap{State: &bldr_plugin.PluginSwap_Failed{Failed: &bldr_plugin.PluginSwapFailed{
		CandidateManifestRef: deployed, CandidateManifestRev: 2, ErrorMessage: "prepare plugin: plugin exited before completing startup",
		FailedAt: deployClock(t, "21:41:02"), RetryAt: deployClock(t, "21:41:03"),
	}}}
	waiting := &bldr_plugin.PluginSwap{State: &bldr_plugin.PluginSwap_Waiting{Waiting: &bldr_plugin.PluginSwapWaiting{
		CandidateManifestRef: deployed, CandidateManifestRev: 2, SinceAt: deployClock(t, "21:41:04"),
	}}}
	settled := &bldr_plugin.PluginSwap{State: &bldr_plugin.PluginSwap_Settled{Settled: &bldr_plugin.PluginSwapSettled{}}}
	start := &s4wave_space.SpaceContentsState{Plugins: []*s4wave_space.SpacePluginStatus{
		{PluginId: "colors", ManifestRef: older, ManifestRev: 1, RunningManifestRef: older, RunningManifestRev: 1, Swap: settled},
	}}
	recv := deployStates(io.EOF,
		row(deployed, settled, older, 1),
		row(deployed, preparing, older, 1),
		row(deployed, preparing, older, 1),
		row(deployed, failed, older, 1),
		row(deployed, preparing, older, 1),
		row(deployed, waiting, older, 1),
		row(deployed, settled, deployed, 2),
	)

	// Each state prints once and live returns success.
	var out strings.Builder
	if err := followDeploy(context.Background(), &out, start, recv, set, time.UTC); err != nil {
		t.Fatal(err)
	}
	want := "waiting for the host to select the deployed manifest (rev 1 selected)\n" +
		"rev 2 selected\n" +
		"preparing rev 2 since 21:41:00\n" +
		"rev 2 failed at 21:41:02: prepare plugin: plugin exited before completing startup; retry at 21:41:03\n" +
		"preparing rev 2 since 21:41:00\n" +
		"rev 2 waits since 21:41:04 for the running worker (rev 1) to answer its activation check\n" +
		"rev 2 live\n"
	if out.String() != want {
		t.Fatalf("follow output:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestFollowDeployStopsStates(t *testing.T) {
	// A failure with no retry ends the follow with the error and next step.
	set, deployed, older := deployFixture(t)
	failed := &s4wave_space.SpacePluginStatus{
		PluginId: "colors", ManifestRef: deployed, ManifestRev: 2, RunningManifestRef: older, RunningManifestRev: 1,
		Swap: &bldr_plugin.PluginSwap{State: &bldr_plugin.PluginSwap_Failed{Failed: &bldr_plugin.PluginSwapFailed{
			CandidateManifestRef: deployed, CandidateManifestRev: 2, ErrorMessage: "startup failed", FailedAt: deployClock(t, "21:42:00"),
		}}},
	}
	var out strings.Builder
	err := followDeploy(context.Background(), &out, &s4wave_space.SpaceContentsState{}, deployStates(io.EOF, failed), set, time.UTC)
	if err == nil || out.String() != "colors is not listed in the space\n"+
		"rev 2 failed at 21:42:00: startup failed\nnext: fix the error and deploy again\n" {
		t.Fatalf("failed follow: %v\n%s", err, out.String())
	}

	// A passed --wait deadline prints the last state and its next step.
	waiting := &s4wave_space.SpacePluginStatus{
		PluginId: "colors", ManifestRef: deployed, ManifestRev: 2, RunningManifestRef: older, RunningManifestRev: 1,
		Swap: &bldr_plugin.PluginSwap{State: &bldr_plugin.PluginSwap_Waiting{Waiting: &bldr_plugin.PluginSwapWaiting{
			CandidateManifestRef: deployed, CandidateManifestRev: 2, SinceAt: deployClock(t, "21:43:00"),
		}}},
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errDeployWaitPassed)
	out.Reset()
	start := &s4wave_space.SpaceContentsState{Plugins: []*s4wave_space.SpacePluginStatus{waiting}}
	err = followDeploy(ctx, &out, start, deployStates(context.Canceled), set, time.UTC)
	if err == nil || !strings.Contains(err.Error(), "--wait deadline") {
		t.Fatalf("deadline follow error: %v", err)
	}
	line := "rev 2 waits since 21:43:00 for the running worker (rev 1) to answer its activation check"
	if out.String() != line+"\nnot live yet: "+line+"\nnext: "+swapWaitingNextStep+"\n" {
		t.Fatalf("deadline follow output:\n%s", out.String())
	}

	// A deploy of the build already running warns and reads live at once.
	live := &s4wave_space.SpacePluginStatus{PluginId: "colors", ManifestRef: deployed, ManifestRev: 2, RunningManifestRef: deployed, RunningManifestRev: 2}
	out.Reset()
	warnDeployOverlap(&out, live, set)
	start = &s4wave_space.SpaceContentsState{Plugins: []*s4wave_space.SpacePluginStatus{live}}
	if err := followDeploy(context.Background(), &out, start, deployStates(io.EOF), set, time.UTC); err != nil {
		t.Fatal(err)
	}
	if out.String() != "warning: rev 2 already runs this build\nrev 2 live\n" {
		t.Fatalf("already live output:\n%s", out.String())
	}

	// Another build of the deployed rev already selected warns.
	other := &s4wave_space.SpacePluginStatus{PluginId: "colors", ManifestRef: older, ManifestRev: 2, RunningManifestRef: older, RunningManifestRev: 2}
	out.Reset()
	warnDeployOverlap(&out, other, set)
	if !strings.HasPrefix(out.String(), "warning: the space already selects another build of rev 2;") {
		t.Fatalf("same rev output:\n%s", out.String())
	}
}
