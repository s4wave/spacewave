package resource_git

import (
	"context"
	stderrors "errors"
	"slices"
	"sort"
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
	repoObjKey := r.snap.RepoObjectKey
	if repoObjKey == "" {
		return nil, errors.New("no linked repo object")
	}

	_, resourceID, err := resource_server.ConstructChildResource(ctx,
		func(subCtx context.Context) (srpc.Invoker, *GitRepoResource, func(), error) {
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
	if !r.snap.HasWorkdir {
		return nil, errors.New("no workdir linked to this worktree")
	}

	_, resourceID, err := resource_server.ConstructChildResource(ctx,
		func(subCtx context.Context) (srpc.Invoker, *unixfs.FSHandle, func(), error) {
			fsCursor, err := unixfs_world.FollowUnixfsRef(subCtx, nil, r.ws, r.snap.WorkdirRef, "", false)
			if err != nil {
				return nil, nil, nil, errors.Wrap(err, "follow workdir ref")
			}

			fsh, err := unixfs.NewFSHandle(fsCursor)
			if err != nil {
				fsCursor.Release()
				return nil, nil, nil, errors.Wrap(err, "create fs handle")
			}

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
	repoObjKey := r.snap.RepoObjectKey
	if repoObjKey == "" {
		return nil, errors.New("no linked repo object")
	}

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
			wt, err := repo.Worktree()
			if err != nil {
				return errors.Wrap(err, "worktree")
			}

			status, err := wt.Status()
			if err != nil {
				return errors.Wrap(err, "status")
			}

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

			sort.Slice(resp.Entries, func(i, j int) bool {
				return resp.Entries[i].GetFilePath() < resp.Entries[j].GetFilePath()
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
	paths := req.GetPaths()
	if len(paths) == 0 {
		return &s4wave_git.StageFilesResponse{}, nil
	}

	repoObjKey := r.snap.RepoObjectKey
	if repoObjKey == "" {
		return nil, errors.New("no linked repo object")
	}

	err := git_world.AccessWorldObjectRepoWithWorktree(
		ctx,
		nil,
		r.ws,
		repoObjKey, r.objKey,
		time.Time{},
		true,
		"",
		func(repo *git.Repository, workDir billy.Filesystem) error {
			wt, err := repo.Worktree()
			if err != nil {
				return errors.Wrap(err, "worktree")
			}
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
	paths := req.GetPaths()
	if len(paths) == 0 {
		return &s4wave_git.UnstageFilesResponse{}, nil
	}

	repoObjKey := r.snap.RepoObjectKey
	if repoObjKey == "" {
		return nil, errors.New("no linked repo object")
	}

	err := git_world.AccessWorldObjectRepoWithWorktree(
		ctx,
		nil,
		r.ws,
		repoObjKey, r.objKey,
		time.Time{},
		true,
		"",
		func(repo *git.Repository, workDir billy.Filesystem) error {
			wt, err := repo.Worktree()
			if err != nil {
				return errors.Wrap(err, "worktree")
			}

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
	message := strings.TrimSpace(req.GetMessage())
	if message == "" {
		return nil, errors.New("commit message cannot be empty")
	}
	authorName := strings.TrimSpace(req.GetAuthorName())
	if authorName == "" {
		return nil, errors.New("author name cannot be empty")
	}
	authorEmail := strings.TrimSpace(req.GetAuthorEmail())
	if authorEmail == "" {
		return nil, errors.New("author email cannot be empty")
	}
	authorTime := time.Now()
	if req.GetAuthorTimestamp() > 0 {
		authorTime = time.Unix(req.GetAuthorTimestamp(), 0)
	}
	paths := cleanPathList(req.GetPaths())
	repoObjKey := r.snap.RepoObjectKey
	if repoObjKey == "" {
		return nil, errors.New("no linked repo object")
	}

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
			headRef, err := repo.Head()
			if err != nil && !stderrors.Is(err, plumbing.ErrReferenceNotFound) {
				return errors.Wrap(err, "head")
			}
			if headRef != nil {
				resp.BaseCommitHash = headRef.Hash().String()
				resp.BranchRef = headRef.Name().Short()
			}

			wt, err := repo.Worktree()
			if err != nil {
				return errors.Wrap(err, "worktree")
			}
			status, err := wt.Status()
			if err != nil {
				return errors.Wrap(err, "status")
			}
			affected, err := stagedPathsForCommit(status, paths)
			if err != nil {
				return err
			}
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
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" || slices.Contains(out, p) {
			continue
		}
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

func stagedPathsForCommit(status git.Status, requested []string) ([]string, error) {
	if len(requested) != 0 {
		out := make([]string, 0, len(requested))
		for _, p := range requested {
			fileStatus, ok := status[p]
			if !ok || !isIndexStaged(fileStatus.Staging) {
				return nil, errors.Errorf("path is not staged: %s", p)
			}
			out = append(out, p)
		}
		for path, fileStatus := range status {
			if !isIndexStaged(fileStatus.Staging) || slices.Contains(requested, path) {
				continue
			}
			return nil, errors.Errorf("unexpected staged path: %s", path)
		}
		return out, nil
	}
	out := make([]string, 0, len(status))
	for path, fileStatus := range status {
		if isIndexStaged(fileStatus.Staging) {
			out = append(out, path)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no staged paths to commit")
	}
	slices.Sort(out)
	return out, nil
}

func isIndexStaged(sc git.StatusCode) bool {
	return sc != git.Unmodified && sc != git.Untracked
}

// _ is a type assertion
var _ s4wave_git.SRPCGitWorktreeResourceServiceServer = (*GitWorktreeResource)(nil)
