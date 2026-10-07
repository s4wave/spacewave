package resource_space

import (
	"context"
	"regexp"
	"strings"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_lib_git_clone "github.com/s4wave/spacewave/forge/lib/git/clone"
	forge_target "github.com/s4wave/spacewave/forge/target"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	uuid "github.com/satori/go.uuid"
)

// pluginRepositoryTaskName names the single Task of a repository fetch Job.
const pluginRepositoryTaskName = "fetch"

// pluginRepositoryKeyPrefix prefixes the git repository object of each
// GitHub repository; its worktree and workdir sit beneath it.
const pluginRepositoryKeyPrefix = "plugin-repos/"

// githubRepositoryPattern matches a GitHub owner and repository name.
var githubRepositoryPattern = regexp.MustCompile(`^([A-Za-z0-9](?:[A-Za-z0-9-]{0,38}))/([A-Za-z0-9._-]{1,100})$`)

// parseGitHubRepository returns the lowercase owner/repo of a GitHub repository
// given as owner/repo or as its https://github.com URL.
func parseGitHubRepository(repository string) (string, error) {
	// Strip the URL form down to owner/repo.
	name := strings.TrimSpace(repository)
	name = strings.TrimPrefix(name, "https://")
	name = strings.TrimPrefix(name, "github.com/")
	name = strings.TrimSuffix(strings.TrimSuffix(name, "/"), ".git")

	// Require a valid owner and a repository name other than a dot path.
	match := githubRepositoryPattern.FindStringSubmatch(name)
	if match == nil || match[2] == "." || match[2] == ".." {
		return "", errors.Errorf("%q is not a GitHub owner/repo", repository)
	}
	return strings.ToLower(name), nil
}

// FetchPluginRepository queues a Forge Job that clones a GitHub repository at
// depth one into the Space, or fetches its newest commit when the repository
// object already exists. The clone handler decides which, so adding and
// checking for updates are the same call.
func (r *SpaceResource) FetchPluginRepository(ctx context.Context, req *s4wave_space.FetchPluginRepositoryRequest) (*s4wave_space.FetchPluginRepositoryResponse, error) {
	// Name the repository objects from the GitHub repository.
	name, err := parseGitHubRepository(req.GetRepository())
	if err != nil {
		return nil, err
	}
	repoKey := pluginRepositoryKeyPrefix + name
	worktreeKey := repoKey + "/worktree"

	// Open the transaction that commits the Job and its checks together.
	tx, err := r.space.GetWorldEngine().NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Place the Job on the selected Device's Worker and Cluster.
	selection, err := r.selectDevicePlacement(ctx, tx, req.GetDeviceKey())
	if err != nil {
		return nil, err
	}
	clusterKey, err := selectWorkerCluster(ctx, tx, selection.placement.GetWorkerObjectKey(), req.GetClusterKey())
	if err != nil {
		return nil, err
	}

	// Clone only the default branch's newest commit, without tags, and check
	// it out into a worktree.
	configData, err := (&forge_lib_git_clone.Config{
		ObjectKey: repoKey,
		CloneOpts: &git_block.CloneOpts{
			Url:          "https://github.com/" + name + ".git",
			Depth:        1,
			SingleBranch: true,
			TagMode:      git_block.TagMode_TagMode_NONE,
		},
		WorktreeOpts: &git_world.GitCreateWorktreeOp{
			ObjectKey: worktreeKey,
			WorkdirRef: &unixfs_world.UnixfsRef{
				ObjectKey: repoKey + "/workdir",
				FsType:    unixfs_world.FSType_FSType_FS_NODE,
			},
			CreateWorkdir: true,
		},
	}).MarshalJSON()
	if err != nil {
		return nil, err
	}
	target := &forge_target.Target{
		Exec: &forge_target.Exec{
			Controller: &configset_proto.ControllerConfig{Id: space_exec.GitCloneConfigID, Rev: 1, Config: configData},
		},
		Outputs: []*forge_target.Output{
			{Name: "repo", OutputType: forge_target.OutputType_OutputType_EXEC, ExecOutput: "repo"},
		},
	}

	// Create the Job on the selected Worker and assign it to the Cluster.
	jobKey := "plugin-repo-fetches/" + uuid.NewV4().String()
	tasks := map[string]*forge_target.Target{pluginRepositoryTaskName: target}
	job, _, err := forge_job.CreateJobWithTasks(ctx, tx, selection.sender, jobKey, tasks, "", selection.placement, timestamppb.Now())
	world.ReleaseObjectState(job)
	if err != nil {
		return nil, err
	}
	if _, _, err := forge_cluster.AssignJobToCluster(ctx, tx, clusterKey, jobKey, selection.sender); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &s4wave_space.FetchPluginRepositoryResponse{
		RepoKey:     repoKey,
		WorktreeKey: worktreeKey,
		JobKey:      jobKey,
		TaskKey:     forge_job.NewJobTaskKey(jobKey, pluginRepositoryTaskName),
	}, nil
}
