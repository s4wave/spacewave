package resource_git

import (
	"context"
	stderrors "errors"
	"slices"
	"strings"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_unixfs "github.com/s4wave/spacewave/core/resource/unixfs"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	s4wave_git "github.com/s4wave/spacewave/sdk/git"
)

// WorktreeSnapshot holds a snapshot of worktree state collected during factory init.
type WorktreeSnapshot struct {
	// RepoObjectKey is the object key of the linked git/repo.
	RepoObjectKey string
	// WorkdirObjectKey is the object key of the linked workdir.
	WorkdirObjectKey string
	// WorkdirRef is the unixfs reference to the workdir.
	WorkdirRef *unixfs_world.UnixfsRef
	// CheckedOutRef is the short name of the checked-out ref.
	CheckedOutRef string
	// HeadCommitHash is the commit hash of the checked-out ref.
	HeadCommitHash string
	// HasWorkdir is true if a workdir is linked.
	HasWorkdir bool
}

// GitWorktreeResource implements GitWorktreeResourceService for a git/worktree object.
type GitWorktreeResource struct {
	ws     world.WorldState
	engine world.Engine
	objKey string
	snap   *WorktreeSnapshot
	mux    srpc.Mux
}

// NewGitWorktreeResource creates a new GitWorktreeResource.
func NewGitWorktreeResource(ws world.WorldState, engine world.Engine, objKey string, snap *WorktreeSnapshot) *GitWorktreeResource {
	r := &GitWorktreeResource{ws: ws, engine: engine, objKey: objKey, snap: snap}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_git.SRPCRegisterGitWorktreeResourceService(mux, r)
	})
	return r
}

// GetMux returns the srpc mux for this resource.
func (r *GitWorktreeResource) GetMux() srpc.Mux {
	return r.mux
}

// GetWorktreeInfo returns worktree metadata.
func (r *GitWorktreeResource) GetWorktreeInfo(ctx context.Context, req *s4wave_git.GetWorktreeInfoRequest) (*s4wave_git.GetWorktreeInfoResponse, error) {
	return &s4wave_git.GetWorktreeInfoResponse{
		RepoObjectKey:    r.snap.RepoObjectKey,
		WorkdirObjectKey: r.snap.WorkdirObjectKey,
		CheckedOutRef:    r.snap.CheckedOutRef,
		HeadCommitHash:   r.snap.HeadCommitHash,
		HasWorkdir:       r.snap.HasWorkdir,
	}, nil
}

// GetRepoResource creates a GitRepoResource sub-resource for the linked git/repo object.
func (r *GitWorktreeResource) GetRepoResource(ctx context.Context, req *s4wave_git.GetRepoResourceRequest) (*s4wave_git.GetRepoResourceResponse, error) {
	// Require a linked repository before constructing its child resource.
	repoObjKey := r.snap.RepoObjectKey
	if repoObjKey == "" {
		return nil, errors.New("no linked repo object")
	}

	// Create a repository resource whose snapshot uses the child context.
	_, resourceID, err := resource_server.ConstructChildResource(ctx,
		func(subCtx context.Context) (srpc.Invoker, *GitRepoResource, func(), error) {
			// Read the linked repository into the child resource snapshot.
			var repoSnap RepoSnapshot
			_, _, err := git_world.AccessWorldObjectRepo(
				subCtx, r.ws, repoObjKey, false,
				nil, nil, nil,
				func(repo *git.Repository) error {
					return SnapshotRepo(repo, &repoSnap)
				},
			)
			if err != nil {
				return nil, nil, nil, errors.Wrap(err, "access repo")
			}

			// Expose the repository snapshot through the child resource mux.
			resource := NewGitRepoResource(r.ws, repoObjKey, &repoSnap)
			return resource.GetMux(), resource, func() {}, nil
		},
	)
	if err != nil {
		return nil, err
	}

	return &s4wave_git.GetRepoResourceResponse{
		ResourceId: resourceID,
	}, nil
}

// GetWorkdirResource creates a FSHandle sub-resource for the mutable workdir.
func (r *GitWorktreeResource) GetWorkdirResource(ctx context.Context, req *s4wave_git.GetWorkdirResourceRequest) (*s4wave_git.GetWorkdirResourceResponse, error) {
	// Require a linked workdir before constructing its filesystem resource.
	if !r.snap.HasWorkdir {
		return nil, errors.New("no workdir linked to this worktree")
	}

	// Create a workdir resource whose handle uses the child context.
	_, resourceID, err := resource_server.ConstructChildResource(ctx,
		func(subCtx context.Context) (srpc.Invoker, *unixfs.FSHandle, func(), error) {
			// Open the linked workdir cursor for the child resource lifetime.
			fsCursor, err := unixfs_world.FollowUnixfsRef(subCtx, nil, r.ws, r.snap.WorkdirRef, "", false)
			if err != nil {
				return nil, nil, nil, errors.Wrap(err, "follow workdir ref")
			}

			// Wrap the workdir cursor in a filesystem handle.
			fsh, err := unixfs.NewFSHandle(fsCursor)
			if err != nil {
				fsCursor.Release()
				return nil, nil, nil, errors.Wrap(err, "create fs handle")
			}

			// Expose the workdir handle and release it with the child resource.
			childMux := resource_unixfs.NewFSHandleResource(fsh).GetMux()
			return childMux, fsh, func() {
				fsh.Release()
			}, nil
		},
	)
	if err != nil {
		return nil, err
	}

	return &s4wave_git.GetWorkdirResourceResponse{
		ResourceId: resourceID,
	}, nil
}

// WatchStatus streams git index state as the worktree changes.
func (r *GitWorktreeResource) WatchStatus(
	req *s4wave_git.WatchStatusRequest,
	strm s4wave_git.SRPCGitWorktreeResourceService_WatchStatusStream,
) error {
	ctx := strm.Context()
	var prev *s4wave_git.WatchStatusResponse
	for {
		seqno, err := r.ws.GetSeqno(ctx)
		if err != nil {
			return err
		}
		resp, err := r.statusSnapshot(ctx)
		if err != nil {
			return err
		}
		if prev == nil || !prev.EqualVT(resp) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp.CloneVT()
		}
		if _, err := r.ws.WaitSeqno(ctx, seqno+1); err != nil {
			return err
		}
	}
}

func (r *GitWorktreeResource) statusSnapshot(ctx context.Context) (*s4wave_git.WatchStatusResponse, error) {
	// Require a linked repository before reading worktree status.
	repoObjKey := r.snap.RepoObjectKey
	if repoObjKey == "" {
		return nil, errors.New("no linked repo object")
	}

	// Read the current worktree status into the stream response.
	resp := &s4wave_git.WatchStatusResponse{}
	err := git_world.AccessWorldObjectRepoWithWorktree(
		ctx,
		nil,
		r.ws,
		repoObjKey, r.objKey,
		time.Time{},
		false,
		"",
		func(repo *git.Repository, workDir billy.Filesystem) error {
			// Open the repository worktree for status inspection.
			wt, err := repo.Worktree()
			if err != nil {
				return errors.Wrap(err, "worktree")
			}

			// Read the staging and workdir status of each path.
			status, err := wt.Status()
			if err != nil {
				return errors.Wrap(err, "status")
			}

			// Collect paths with staging or workdir changes.
			for path, fs := range status {
				if fs.Staging == git.Unmodified && fs.Worktree == git.Unmodified {
					continue
				}
				resp.Entries = append(resp.Entries, &s4wave_git.StatusEntry{
					FilePath:       path,
					StagingStatus:  mapStatusCode(fs.Staging),
					WorktreeStatus: mapStatusCode(fs.Worktree),
				})
			}

			// Order the status entries by file path.
			slices.SortFunc(resp.Entries, func(a, b *s4wave_git.StatusEntry) int {
				return strings.Compare(a.GetFilePath(), b.GetFilePath())
			})

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// StageFiles stages files in the git index.
func (r *GitWorktreeResource) StageFiles(ctx context.Context, req *s4wave_git.StageFilesRequest) (*s4wave_git.StageFilesResponse, error) {
	// Finish empty staging requests without opening the repository.
	paths := req.GetPaths()
	if len(paths) == 0 {
		return &s4wave_git.StageFilesResponse{}, nil
	}

	// Require the linked repository before staging files.
	repoObjKey := r.snap.RepoObjectKey
	if repoObjKey == "" {
		return nil, errors.New("no linked repo object")
	}

	// Stage the requested paths in a writable worktree transaction.
	err := git_world.AccessWorldObjectRepoWithWorktree(
		ctx,
		nil,
		r.ws,
		repoObjKey, r.objKey,
		time.Time{},
		true,
		"",
		func(repo *git.Repository, workDir billy.Filesystem) error {
			// Open the repository worktree for staging.
			wt, err := repo.Worktree()
			if err != nil {
				return errors.Wrap(err, "worktree")
			}

			// Add each requested path to the worktree index.
			for _, p := range paths {
				if _, err := wt.Add(p); err != nil {
					return errors.Wrap(err, "stage "+p)
				}
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return &s4wave_git.StageFilesResponse{}, nil
}

// UnstageFiles unstages files from the git index.
func (r *GitWorktreeResource) UnstageFiles(ctx context.Context, req *s4wave_git.UnstageFilesRequest) (*s4wave_git.UnstageFilesResponse, error) {
	// Finish empty unstaging requests without opening the repository.
	paths := req.GetPaths()
	if len(paths) == 0 {
		return &s4wave_git.UnstageFilesResponse{}, nil
	}

	// Require the linked repository before unstaging files.
	repoObjKey := r.snap.RepoObjectKey
	if repoObjKey == "" {
		return nil, errors.New("no linked repo object")
	}

	// Reset the requested index paths in a writable worktree transaction.
	err := git_world.AccessWorldObjectRepoWithWorktree(
		ctx,
		nil,
		r.ws,
		repoObjKey, r.objKey,
		time.Time{},
		true,
		"",
		func(repo *git.Repository, workDir billy.Filesystem) error {
			// Open the repository worktree for the index reset.
			wt, err := repo.Worktree()
			if err != nil {
				return errors.Wrap(err, "worktree")
			}

			// Resolve HEAD as the source for the index reset.
			headRef, err := repo.Head()
			if err != nil {
				return errors.Wrap(err, "head")
			}

			return wt.Reset(&git.ResetOptions{
				Commit: headRef.Hash(),
				Files:  paths,
			})
		},
	)
	if err != nil {
		return nil, err
	}

	return &s4wave_git.UnstageFilesResponse{}, nil
}

// CommitFiles commits staged files in the git index.
func (r *GitWorktreeResource) CommitFiles(ctx context.Context, req *s4wave_git.CommitFilesRequest) (*s4wave_git.CommitFilesResponse, error) {
	// Require a nonempty message for the staged commit.
	message := strings.TrimSpace(req.GetMessage())
	if message == "" {
		return nil, errors.New("commit message cannot be empty")
	}

	// Require the commit author name.
	authorName := strings.TrimSpace(req.GetAuthorName())
	if authorName == "" {
		return nil, errors.New("author name cannot be empty")
	}

	// Require the commit author email.
	authorEmail := strings.TrimSpace(req.GetAuthorEmail())
	if authorEmail == "" {
		return nil, errors.New("author email cannot be empty")
	}

	// Choose the commit author time from the request or the current clock.
	authorTime := time.Now()
	if req.GetAuthorTimestamp() > 0 {
		authorTime = time.Unix(req.GetAuthorTimestamp(), 0)
	}

	// Normalize the commit paths and require the linked repository.
	paths := cleanPathList(req.GetPaths())
	repoObjKey := r.snap.RepoObjectKey
	if repoObjKey == "" {
		return nil, errors.New("no linked repo object")
	}

	// Commit the staged paths and collect the resulting commit metadata.
	resp := &s4wave_git.CommitFilesResponse{}
	err := git_world.AccessWorldObjectRepoWithWorktree(
		ctx,
		nil,
		r.ws,
		repoObjKey, r.objKey,
		time.Time{},
		true,
		"",
		func(repo *git.Repository, workDir billy.Filesystem) error {
			// Record the current HEAD as the commit base and branch.
			headRef, err := repo.Head()
			if err != nil && !stderrors.Is(err, plumbing.ErrReferenceNotFound) {
				return errors.Wrap(err, "head")
			}
			if headRef != nil {
				resp.BaseCommitHash = headRef.Hash().String()
				resp.BranchRef = headRef.Name().Short()
			}

			// Open the repository worktree for the staged commit.
			wt, err := repo.Worktree()
			if err != nil {
				return errors.Wrap(err, "worktree")
			}

			// Read the worktree index before selecting staged paths.
			status, err := wt.Status()
			if err != nil {
				return errors.Wrap(err, "status")
			}

			// Validate the requested paths against the complete staged index.
			affected, err := stagedPathsForCommit(status, paths)
			if err != nil {
				return err
			}

			// Create the commit with the requested message and author.
			hash, err := wt.Commit(message, &git.CommitOptions{
				Author: &object.Signature{
					Name:  authorName,
					Email: authorEmail,
					When:  authorTime,
				},
			})
			if err != nil {
				return errors.Wrap(err, "commit")
			}

			// Record the new commit hash and affected paths.
			resp.CommitHash = hash.String()
			resp.AffectedPaths = affected
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// mapStatusCode maps a go-git StatusCode to a proto FileStatusCode.
func mapStatusCode(sc git.StatusCode) s4wave_git.FileStatusCode {
	switch sc {
	case git.Unmodified:
		return s4wave_git.FileStatusCode_FILE_STATUS_CODE_UNMODIFIED
	case git.Untracked:
		return s4wave_git.FileStatusCode_FILE_STATUS_CODE_UNTRACKED
	case git.Modified:
		return s4wave_git.FileStatusCode_FILE_STATUS_CODE_MODIFIED
	case git.Added:
		return s4wave_git.FileStatusCode_FILE_STATUS_CODE_ADDED
	case git.Deleted:
		return s4wave_git.FileStatusCode_FILE_STATUS_CODE_DELETED
	case git.Renamed:
		return s4wave_git.FileStatusCode_FILE_STATUS_CODE_RENAMED
	case git.Copied:
		return s4wave_git.FileStatusCode_FILE_STATUS_CODE_COPIED
	case git.UpdatedButUnmerged:
		return s4wave_git.FileStatusCode_FILE_STATUS_CODE_UPDATED_BUT_UNMERGED
	default:
		return s4wave_git.FileStatusCode_FILE_STATUS_CODE_UNMODIFIED
	}
}

func cleanPathList(paths []string) []string {
	// Discard empty path lists before normalizing commit selections.
	if len(paths) == 0 {
		return nil
	}

	// Trim path names and keep each nonempty path once.
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" || slices.Contains(out, p) {
			continue
		}
		out = append(out, p)
	}

	// Order the normalized commit paths.
	slices.Sort(out)
	return out
}

func stagedPathsForCommit(status git.Status, requested []string) ([]string, error) {
	// Validate explicitly requested paths against the staged index.
	if len(requested) != 0 {
		// Require each requested path to have a staged index change.
		out := make([]string, 0, len(requested))
		for _, p := range requested {
			fileStatus, ok := status[p]
			if !ok || !isIndexStaged(fileStatus.Staging) {
				return nil, errors.Errorf("path is not staged: %s", p)
			}
			out = append(out, p)
		}

		// Reject staged paths omitted from the explicit commit selection.
		for path, fileStatus := range status {
			if !isIndexStaged(fileStatus.Staging) || slices.Contains(requested, path) {
				continue
			}
			return nil, errors.Errorf("unexpected staged path: %s", path)
		}
		return out, nil
	}

	// Collect all staged paths when the request has no explicit selection.
	out := make([]string, 0, len(status))
	for path, fileStatus := range status {
		if isIndexStaged(fileStatus.Staging) {
			out = append(out, path)
		}
	}

	// Require at least one staged path before creating a commit.
	if len(out) == 0 {
		return nil, errors.New("no staged paths to commit")
	}

	// Order the staged paths in the commit response.
	slices.Sort(out)
	return out, nil
}

func isIndexStaged(sc git.StatusCode) bool {
	return sc != git.Unmodified && sc != git.Untracked
}

var _ s4wave_git.SRPCGitWorktreeResourceServiceServer = (*GitWorktreeResource)(nil)
