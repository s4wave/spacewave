package resource_space

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
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

// pluginRepositoryFiles is a JavaScript plugin repository that validates.
var pluginRepositoryFiles = map[string]string{
	"bldr.star": `project(id="acme")
manifest("acme-colors",
    builder="bldr/plugin/compiler/js",
    description="Color swatches",
    config={"modules": [js_module("JS_MODULE_KIND_BACKEND", "./backend.ts", entrypoint=True)]},
)
`,
	"backend.ts":   "export default {}\n",
	"package.json": `{"name": "acme-colors"}`,
}

// TestValidatePluginRepository validates a fetched repository's checked-out
// commit and builds it only at that commit.
func TestValidatePluginRepository(t *testing.T) {
	// Create a local source repository.
	dir := t.TempDir()
	src, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	srcWt, err := src.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	// Commit the plugin repository's files.
	for name, body := range pluginRepositoryFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := srcWt.Add(name); err != nil {
			t.Fatal(err)
		}
	}
	sig := &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()}
	commit, err := srcWt.Commit("plugin", &git.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		t.Fatal(err)
	}

	// Start a testbed with a build Device and fetch the repository into it.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	peer := tb.Volume.GetPeerID()
	build := setupPluginBuildSource(t, tb, peer)
	fetch := &forge_lib_git_clone.Config{
		ObjectKey: "plugin-repos/acme/colors",
		CloneOpts: &git_block.CloneOpts{Url: (&url.URL{Scheme: "file", Path: dir}).String(), Depth: 1, SingleBranch: true},
		WorktreeOpts: &git_world.GitCreateWorktreeOp{
			ObjectKey:     "plugin-repos/acme/colors/worktree",
			WorkdirRef:    &unixfs_world.UnixfsRef{ObjectKey: "plugin-repos/acme/colors/workdir", FsType: unixfs_world.FSType_FSType_FS_NODE},
			CreateWorkdir: true,
		},
	}
	if _, err := fetch.CloneOrFetch(ctx, tb.Logger, tb.WorldState, peer, timestamp.Now(), nil, nil); err != nil {
		t.Fatal(err)
	}

	// Validate the checked-out commit through the public RPC.
	body := &spaceResourceChatBody{engine: tb.BusEngine, engineID: tb.EngineID, bucketID: tb.EngineBucketID}
	resource := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, peer.String())
	client := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, resource.GetMux()))
	response, err := client.ValidatePluginRepository(ctx, &s4wave_space.ValidatePluginRepositoryRequest{Repository: "acme/colors"})
	if err != nil {
		t.Fatal(err)
	}
	validation := response.GetValidation()
	if response.GetCommit() != commit.String() || response.GetSourceKey() != "plugin-repos/acme/colors/workdir" ||
		len(validation.GetRefusals()) != 0 || len(validation.GetPlugins()) != 1 || validation.GetPlugins()[0].GetManifestId() != "acme-colors" {
		t.Fatalf("unexpected validation: %v", response)
	}

	// Refuse a build at another commit, from another config or of another plugin.
	build.SourceKey, build.ManifestId, build.Commit = response.GetSourceKey(), "acme-colors", response.GetCommit()
	refusals := []struct {
		edit func(*s4wave_space.BuildSpacePluginRequest)
		want string
	}{
		{func(r *s4wave_space.BuildSpacePluginRequest) { r.Commit = strings.Repeat("0", 40) }, "review it before building"},
		{func(r *s4wave_space.BuildSpacePluginRequest) { r.ConfigPath = "bldr.yaml" }, "builds from its bldr.star"},
		{func(r *s4wave_space.BuildSpacePluginRequest) { r.ManifestId = "acme-other" }, "does not declare plugin"},
		{func(r *s4wave_space.BuildSpacePluginRequest) { r.SourceKey = "plugin-repos/acme/colors/worktree" }, "builds from its workdir"},
	}
	for _, refusal := range refusals {
		bad := build.CloneVT()
		refusal.edit(bad)
		if _, err := client.BuildSpacePlugin(ctx, bad); err == nil || !strings.Contains(err.Error(), refusal.want) {
			t.Fatalf("expected a refusal containing %q, got %v", refusal.want, err)
		}
	}

	// Queue the build at the reviewed commit.
	if _, err := client.BuildSpacePlugin(ctx, build); err != nil {
		t.Fatal(err)
	}
}
