package resource_space

import (
	"testing"

	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	git_block "github.com/s4wave/spacewave/db/git/block"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_lib_git_clone "github.com/s4wave/spacewave/forge/lib/git/clone"
	forge_task "github.com/s4wave/spacewave/forge/task"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/s4wave/spacewave/testbed"
)

// TestParseGitHubRepository checks the accepted owner/repo forms.
func TestParseGitHubRepository(t *testing.T) {
	for input, want := range map[string]string{
		"s4wave/Spreadsheet":                         "s4wave/spreadsheet",
		" https://github.com/s4wave/spreadsheet.git": "s4wave/spreadsheet",
		"github.com/s4wave/spreadsheet/":             "s4wave/spreadsheet",
		"s4wave/..":                                  "",
		"s4wave":                                     "",
		"-s4wave/spreadsheet":                        "",
		"s4wave/spreadsheet/tree/main":               "",
		"https://gitlab.com/s4wave/spreadsheet":      "",
	} {
		got, err := parseGitHubRepository(input)
		if got != want || (err == nil) != (want != "") {
			t.Errorf("parseGitHubRepository(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

// TestQueuePluginRepositoryFetch checks the public RPC queues a depth-one,
// single-branch, tagless clone Job on the selected Device.
func TestQueuePluginRepositoryFetch(t *testing.T) {
	// Start a testbed with a Device whose Worker belongs to one Cluster.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	peer := tb.Volume.GetPeerID()
	setupPluginBuildSource(t, tb, peer)

	// Mount the Space Resource as the submitting Session and queue the fetch.
	body := &spaceResourceChatBody{engine: tb.BusEngine, engineID: tb.EngineID, bucketID: tb.EngineBucketID}
	resource := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, peer.String())
	client := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, resource.GetMux()))
	request := &s4wave_space.FetchPluginRepositoryRequest{Repository: "s4wave/spreadsheet", DeviceKey: "devices/build"}
	response, err := client.FetchPluginRepository(ctx, request)
	if err != nil {
		t.Fatal(err)
	}

	// The Job runs on the selected Worker and belongs to its Cluster.
	job, err := forge_job.LookupJobBody(ctx, tb.WorldState, response.GetJobKey())
	if err != nil {
		t.Fatal(err)
	}
	if job.GetPlacement().GetWorkerObjectKey() != "workers/build" {
		t.Fatal("queued job lost its selected device")
	}
	assigned, err := forge_cluster.CheckClusterHasJob(ctx, tb.WorldState, "clusters/build", response.GetJobKey())
	if err != nil || !assigned {
		t.Fatalf("job is not assigned to its cluster: %v", err)
	}

	// The Task runs the git clone handler at depth one without tags.
	target, _, err := forge_task.LookupTaskTarget(ctx, tb.WorldState, response.GetTaskKey())
	if err != nil {
		t.Fatal(err)
	}
	controller := target.GetExec().GetController()
	if controller.GetId() != space_exec.GitCloneConfigID {
		t.Fatalf("queued task runs %q", controller.GetId())
	}
	var config forge_lib_git_clone.Config
	if err := config.UnmarshalJSON(controller.GetConfig()); err != nil {
		t.Fatal(err)
	}
	clone := config.GetCloneOpts()
	if config.GetObjectKey() != "plugin-repos/s4wave/spreadsheet" || config.GetWorktreeOpts().GetObjectKey() != "plugin-repos/s4wave/spreadsheet/worktree" ||
		clone.GetUrl() != "https://github.com/s4wave/spreadsheet.git" || clone.GetDepth() != 1 || !clone.GetSingleBranch() ||
		clone.GetTagMode() != git_block.TagMode_TagMode_NONE {
		t.Fatalf("queued task config is not a depth-one clone: %v", &config)
	}

	// Invalid repositories and non-Device placements cannot enqueue work.
	for _, bad := range []*s4wave_space.FetchPluginRepositoryRequest{
		{Repository: "s4wave/..", DeviceKey: "devices/build"},
		{Repository: "s4wave/spreadsheet", DeviceKey: "projects/colors"},
		{Repository: "s4wave/spreadsheet", DeviceKey: "devices/build", ClusterKey: "clusters/other"},
	} {
		if _, err := client.FetchPluginRepository(ctx, bad); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
}
