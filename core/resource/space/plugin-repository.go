package resource_space

import (
	"context"
	"regexp"
	"slices"
	"strings"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	bldr_project_validate "github.com/s4wave/spacewave/bldr/project/validate"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
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
		JobKey:  jobKey,
		TaskKey: forge_job.NewJobTaskKey(jobKey, pluginRepositoryTaskName),
	}, nil
}

// ValidatePluginRepository validates the commit a fetched plugin repository
// has checked out. It reads the Space World only; no repository code runs.
func (r *SpaceResource) ValidatePluginRepository(ctx context.Context, req *s4wave_space.ValidatePluginRepositoryRequest) (*s4wave_space.ValidatePluginRepositoryResponse, error) {
	// Name the repository objects from the GitHub repository.
	name, err := parseGitHubRepository(req.GetRepository())
	if err != nil {
		return nil, err
	}
	repoKey := pluginRepositoryKeyPrefix + name

	// Validate the checked-out commit in one read transaction.
	tx, err := r.space.GetWorldEngine().NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	commit, validation, err := r.validatePluginRepository(ctx, tx, repoKey)
	if err != nil {
		return nil, err
	}
	return &s4wave_space.ValidatePluginRepositoryResponse{
		Commit:     commit,
		SourceKey:  repoKey + "/workdir",
		Validation: validation,
	}, nil
}

// checkPluginRepositoryBuild admits a build of a plugin repository's workdir
// only at the commit the caller reviewed, when its validation refuses nothing
// and declares the requested plugin. The build reads the same World state.
func (r *SpaceResource) checkPluginRepositoryBuild(ctx context.Context, ws world.WorldState, req *s4wave_space.BuildSpacePluginRequest) error {
	// Require a repository's workdir built from its bldr.star.
	repoKey, ok := strings.CutSuffix(req.GetSourceKey(), "/workdir")
	name := strings.TrimPrefix(repoKey, pluginRepositoryKeyPrefix)
	if parsed, err := parseGitHubRepository(name); !ok || err != nil || parsed != name {
		return errors.New("a plugin repository builds from its workdir")
	}
	if req.GetConfigPath() != "" {
		return errors.New("a plugin repository builds from its bldr.star")
	}

	// Validate the checked-out commit and match it to the review.
	commit, validation, err := r.validatePluginRepository(ctx, ws, repoKey)
	if err != nil {
		return err
	}
	if commit != req.GetCommit() {
		return errors.Errorf("the plugin repository is at commit %s; review it before building", commit)
	}
	if refusals := validation.GetRefusals(); len(refusals) != 0 {
		return errors.New("the plugin repository is refused: " + refusals[0].GetReason())
	}
	if !slices.ContainsFunc(validation.GetPlugins(), func(p *bldr_project_validate.Plugin) bool {
		return p.GetManifestId() == req.GetManifestId()
	}) {
		return errors.Errorf("the plugin repository does not declare plugin %q", req.GetManifestId())
	}
	return nil
}

// validatePluginRepository validates the workdir of the repository at repoKey
// and returns the commit its worktree has checked out.
func (r *SpaceResource) validatePluginRepository(ctx context.Context, ws world.WorldState, repoKey string) (string, *bldr_project_validate.Validation, error) {
	// Read the commit the worktree has checked out.
	worktreeKey := repoKey + "/worktree"
	objectType, err := world_types.GetObjectType(ctx, ws, worktreeKey)
	if err != nil {
		return "", nil, err
	}
	if objectType != git_world.GitWorktreeTypeID {
		return "", nil, errors.New("fetch the plugin repository first")
	}
	head, err := git_world.LookupWorktreeHead(ctx, ws, worktreeKey, repoKey)
	if err != nil {
		return "", nil, err
	}
	if head == nil {
		return "", nil, errors.New("the plugin repository has no commit checked out")
	}

	// Open the workdir as a read-only file system.
	workdirKey := repoKey + "/workdir"
	fsType, _, err := unixfs_world.LookupFsType(ctx, ws, workdirKey)
	if err != nil {
		return "", nil, err
	}
	cursor := unixfs_world.NewFSCursorWithContext(ctx, r.le, ws, workdirKey, fsType, nil, false)
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		cursor.Release()
		return "", nil, err
	}
	defer handle.Release()

	// Validate the project in the workdir.
	validation, err := bldr_project_validate.Validate(ctx, unixfs_iofs.NewFS(ctx, handle))
	if err != nil {
		return "", nil, err
	}
	return head.Hash().String(), validation, nil
}
