package resource_git

import (
	"context"
	"io"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_unixfs "github.com/s4wave/spacewave/core/resource/unixfs"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_unixfs "github.com/s4wave/spacewave/db/git/unixfs"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/world"
	s4wave_git "github.com/s4wave/spacewave/sdk/git"
	git_repofs "github.com/s4wave/spacewave/sdk/git/repofs"
)

// RepoSnapshot holds a snapshot of repo state collected during factory init.
type RepoSnapshot struct {
	// HeadRef is the short name of HEAD's target reference.
	HeadRef string
	// HeadCommitHash is the commit hash HEAD resolves to.
	HeadCommitHash string
	// ReadmePath is the path to the README file in the root tree.
	ReadmePath string
	// LastCommit contains info about the HEAD commit.
	LastCommit *CommitSnapshot
	// IsEmpty is true if the repo has no commits.
	IsEmpty bool
	// Branches is the list of branch refs.
	Branches []RefSnapshot
	// Tags is the list of tag refs.
	Tags []RefSnapshot
}

// CommitSnapshot holds snapshot data for a single commit.
type CommitSnapshot struct {
	// Hash is the commit hash.
	Hash string
	// Message is the commit message.
	Message string
	// AuthorName is the author name.
	AuthorName string
	// AuthorEmail is the author email.
	AuthorEmail string
	// AuthorTimestamp is the author time as Unix seconds.
	AuthorTimestamp int64
}

// RefSnapshot holds snapshot data for a single reference.
type RefSnapshot struct {
	// Name is the short reference name.
	Name string
	// CommitHash is the commit hash.
	CommitHash string
	// IsHead is true if this ref matches HEAD.
	IsHead bool
}

// GitRepoResource implements GitRepoResourceService for a git/repo object.
// Each RPC call re-opens the repo via AccessWorldObjectRepo for read-only access.
type GitRepoResource struct {
	ws     world.WorldState
	objKey string
	snap   *RepoSnapshot
	mux    srpc.Mux
}

// NewGitRepoResource creates a new GitRepoResource.
func NewGitRepoResource(ws world.WorldState, objKey string, snap *RepoSnapshot) *GitRepoResource {
	r := &GitRepoResource{ws: ws, objKey: objKey, snap: snap}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_git.SRPCRegisterGitRepoResourceService(mux, r)
	})
	return r
}

// GetMux returns the srpc mux for this resource.
func (r *GitRepoResource) GetMux() srpc.Mux {
	return r.mux
}

// ListRefs lists all branches and tags in the repository.
func (r *GitRepoResource) ListRefs(ctx context.Context, req *s4wave_git.ListRefsRequest) (*s4wave_git.ListRefsResponse, error) {
	resp := &s4wave_git.ListRefsResponse{
		HeadRef: r.snap.HeadRef,
	}

	for _, b := range r.snap.Branches {
		resp.Branches = append(resp.Branches, &s4wave_git.RefInfo{
			Name:       b.Name,
			CommitHash: b.CommitHash,
			IsHead:     b.IsHead,
		})
	}
	for _, t := range r.snap.Tags {
		resp.Tags = append(resp.Tags, &s4wave_git.RefInfo{
			Name:       t.Name,
			CommitHash: t.CommitHash,
		})
	}

	return resp, nil
}

// ResolveRef resolves a ref name to a commit hash and tree hash.
func (r *GitRepoResource) ResolveRef(ctx context.Context, req *s4wave_git.ResolveRefRequest) (*s4wave_git.ResolveRefResponse, error) {
	refName := req.GetRefName()
	if refName == "" {
		return nil, errors.New("ref_name is required")
	}

	resp := &s4wave_git.ResolveRefResponse{}
	_, _, err := git_world.AccessWorldObjectRepo(
		ctx, r.ws, r.objKey, false,
		nil, nil, nil,
		func(repo *git.Repository) error {
			commit, err := resolveRefToCommitObject(repo, refName)
			if err != nil {
				return err
			}
			resp.CommitHash = commit.Hash.String()
			resp.TreeHash = commit.TreeHash.String()
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// GetRepoInfo returns repository overview information.
func (r *GitRepoResource) GetRepoInfo(ctx context.Context, req *s4wave_git.GetRepoInfoRequest) (*s4wave_git.GetRepoInfoResponse, error) {
	resp := &s4wave_git.GetRepoInfoResponse{
		HeadRef:        r.snap.HeadRef,
		HeadCommitHash: r.snap.HeadCommitHash,
		ReadmePath:     r.snap.ReadmePath,
		IsEmpty:        r.snap.IsEmpty,
	}

	if r.snap.LastCommit != nil {
		resp.LastCommit = &s4wave_git.CommitInfo{
			Hash:            r.snap.LastCommit.Hash,
			Message:         r.snap.LastCommit.Message,
			AuthorName:      r.snap.LastCommit.AuthorName,
			AuthorEmail:     r.snap.LastCommit.AuthorEmail,
			AuthorTimestamp: r.snap.LastCommit.AuthorTimestamp,
		}
	}

	return resp, nil
}

// GetTreeResource creates a FSHandle sub-resource for a ref's tree.
//
// Uses ConstructChildResource to get a sub-context that outlives the RPC call.
// The persistent block cursors, git store, and FSCursor all use this sub-context
// and are released when the sub-resource is torn down.
func (r *GitRepoResource) GetTreeResource(ctx context.Context, req *s4wave_git.GetTreeResourceRequest) (*s4wave_git.GetTreeResourceResponse, error) {
	refName := req.GetRefName()

	_, resourceID, err := resource_server.ConstructChildResource(ctx,
		func(subCtx context.Context) (srpc.Invoker, *unixfs.FSHandle, func(), error) {
			// Look up the git repo object in the world state.
			objState, objFound, err := r.ws.GetObject(subCtx, r.objKey)
			if err != nil {
				return nil, nil, nil, err
			}
			if !objFound {
				return nil, nil, nil, errors.New("git repo object not found")
			}
			objRef, _, err := objState.GetRootRef(subCtx)
			if err != nil {
				return nil, nil, nil, errors.Wrap(err, "get root ref")
			}

			// Build persistent cursor chain to the git repo's block storage.
			rootCursor, err := r.ws.BuildStorageCursor(subCtx)
			if err != nil {
				return nil, nil, nil, errors.Wrap(err, "build storage cursor")
			}
			locCursor, err := rootCursor.FollowRef(subCtx, objRef)
			if err != nil {
				rootCursor.Release()
				return nil, nil, nil, errors.Wrap(err, "follow ref")
			}
			_, bcs := locCursor.BuildTransaction(nil)

			// Validate the repo block at the cursor position.
			repob, err := git_block.UnmarshalRepo(subCtx, bcs)
			if err != nil {
				locCursor.Release()
				rootCursor.Release()
				return nil, nil, nil, errors.Wrap(err, "unmarshal repo")
			}
			if err := repob.Validate(); err != nil {
				locCursor.Release()
				rootCursor.Release()
				return nil, nil, nil, errors.Wrap(err, "validate repo")
			}

			// Create a persistent read-only git store (btx=nil).
			// subCtx outlives the RPC -- the store stays alive for subsequent
			// FSHandle operations on the child resource.
			store, err := git_block.NewStore(subCtx, nil, bcs, &memory.IndexStorage{}, nil)
			if err != nil {
				locCursor.Release()
				rootCursor.Release()
				return nil, nil, nil, errors.Wrap(err, "create git store")
			}

			cleanup := func() {
				store.Close()
				locCursor.Release()
				rootCursor.Release()
			}

			// Open git repository for ref resolution.
			repo, err := git.Open(store, memfs.New())
			if errors.Is(err, git.ErrRepositoryNotExists) {
				repo, err = git.Init(store, git.WithWorkTree(memfs.New()))
			}
			if err != nil {
				cleanup()
				return nil, nil, nil, errors.Wrap(err, "open git repo")
			}

			// Resolve ref to tree hash, then load the tree object.
			treeHash, err := resolveRefToTree(repo, refName)
			if err != nil {
				cleanup()
				return nil, nil, nil, err
			}
			tree, err := object.GetTree(store, treeHash)
			if err != nil {
				cleanup()
				return nil, nil, nil, errors.Wrap(err, "get tree")
			}

			// Create git-backed FSCursor with the persistent store.
			fsCursor := git_unixfs.NewGitFSCursor(store, tree, "")
			fsh, err := unixfs.NewFSHandle(fsCursor)
			if err != nil {
				fsCursor.Release()
				cleanup()
				return nil, nil, nil, errors.Wrap(err, "create fs handle")
			}

			childMux := resource_unixfs.NewFSHandleResource(fsh).GetMux()
			return childMux, fsh, func() {
				fsh.Release()
				cleanup()
			}, nil
		},
	)
	if err != nil {
		return nil, err
	}

	return &s4wave_git.GetTreeResourceResponse{
		ResourceId: resourceID,
	}, nil
}

// GetRepoFilesystemResource creates a FSHandle sub-resource for the materialized repo filesystem.
func (r *GitRepoResource) GetRepoFilesystemResource(ctx context.Context, req *s4wave_git.GetRepoFilesystemResourceRequest) (*s4wave_git.GetRepoFilesystemResourceResponse, error) {
	_, resourceID, err := resource_server.ConstructChildResource(ctx,
		func(subCtx context.Context) (srpc.Invoker, *unixfs.FSHandle, func(), error) {
			fsCursor, err := git_repofs.OpenRepoFSCursor(subCtx, r.ws, r.objKey, req.GetWritable())
			if err != nil {
				return nil, nil, nil, err
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

	return &s4wave_git.GetRepoFilesystemResourceResponse{
		ResourceId: resourceID,
	}, nil
}

// Log returns a paginated list of commits starting from a ref.
func (r *GitRepoResource) Log(ctx context.Context, req *s4wave_git.LogRequest) (*s4wave_git.LogResponse, error) {
	refName := req.GetRefName()
	sinceRef := req.GetSinceRef()
	offset := req.GetOffset()
	limit := req.GetLimit()
	if limit == 0 {
		limit = 50
	}

	resp := &s4wave_git.LogResponse{}
	_, _, err := git_world.AccessWorldObjectRepo(
		ctx, r.ws, r.objKey, false,
		nil, nil, nil,
		func(repo *git.Repository) error {
			commitHash, err := resolveRefToCommit(repo, refName)
			if err != nil {
				return err
			}

			// Build exclusion set from since_ref ancestors.
			var excludeSet map[plumbing.Hash]struct{}
			if sinceRef != "" {
				sinceHash, err := resolveRefToCommit(repo, sinceRef)
				if err != nil {
					return errors.Wrap(err, "resolve since_ref")
				}
				excludeSet = make(map[plumbing.Hash]struct{})
				sinceIter, err := repo.Log(&git.LogOptions{From: sinceHash})
				if err != nil {
					return errors.Wrap(err, "log since_ref")
				}
				for {
					c, err := sinceIter.Next()
					if err == io.EOF {
						break
					}
					if err != nil {
						return errors.Wrap(err, "iterate since_ref commits")
					}
					excludeSet[c.Hash] = struct{}{}
				}
			}

			iter, err := repo.Log(&git.LogOptions{From: commitHash})
			if err != nil {
				return errors.Wrap(err, "log")
			}

			var skipped uint32
			var collected uint32
			for {
				commit, err := iter.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					return errors.Wrap(err, "iterate commits")
				}
				if excludeSet != nil {
					if _, excluded := excludeSet[commit.Hash]; excluded {
						continue
					}
				}
				if skipped < offset {
					skipped++
					continue
				}
				if collected >= limit {
					resp.HasMore = true
					break
				}
				resp.Commits = append(resp.Commits, commitToInfo(commit))
				collected++
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// GetCommit returns full metadata for a single commit.
func (r *GitRepoResource) GetCommit(ctx context.Context, req *s4wave_git.GetCommitRequest) (*s4wave_git.GetCommitResponse, error) {
	hash := req.GetHash()
	if hash == "" {
		return nil, errors.New("hash is required")
	}

	resp := &s4wave_git.GetCommitResponse{}
	_, _, err := git_world.AccessWorldObjectRepo(
		ctx, r.ws, r.objKey, false,
		nil, nil, nil,
		func(repo *git.Repository) error {
			commit, err := resolveRefToCommitObject(repo, hash)
			if err != nil {
				return err
			}
			resp.Commit = commitToInfo(commit)
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// GetDiffStat returns diff stats between two refs.
func (r *GitRepoResource) GetDiffStat(ctx context.Context, req *s4wave_git.GetDiffStatRequest) (*s4wave_git.GetDiffStatResponse, error) {
	refA := req.GetRefA()
	if refA == "" {
		return nil, errors.New("ref_a is required")
	}
	refB := req.GetRefB()

	resp := &s4wave_git.GetDiffStatResponse{}
	_, _, err := git_world.AccessWorldObjectRepo(
		ctx, r.ws, r.objKey, false,
		nil, nil, nil,
		func(repo *git.Repository) error {
			patch, err := diffRefs(repo, refA, refB)
			if err != nil {
				return err
			}

			var totalAdd, totalDel uint32
			for _, fs := range patch.Stats() {
				add := uint32(fs.Addition)
				del := uint32(fs.Deletion)
				resp.Files = append(resp.Files, &s4wave_git.DiffFileStat{
					Path:      fs.Name,
					Additions: add,
					Deletions: del,
				})
				totalAdd += add
				totalDel += del
			}
			resp.TotalAdditions = totalAdd
			resp.TotalDeletions = totalDel
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// GetDiffPatch returns a unified diff patch between two refs.
func (r *GitRepoResource) GetDiffPatch(ctx context.Context, req *s4wave_git.GetDiffPatchRequest) (*s4wave_git.GetDiffPatchResponse, error) {
	refA := req.GetRefA()
	if refA == "" {
		return nil, errors.New("ref_a is required")
	}
	refB := req.GetRefB()

	resp := &s4wave_git.GetDiffPatchResponse{}
	_, _, err := git_world.AccessWorldObjectRepo(
		ctx, r.ws, r.objKey, false,
		nil, nil, nil,
		func(repo *git.Repository) error {
			patch, err := diffRefs(repo, refA, refB)
			if err != nil {
				return err
			}
			resp.Patch = patch.String()
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func diffRefs(repo *git.Repository, refA, refB string) (*object.Patch, error) {
	commitA, err := resolveRefToCommitObject(repo, refA)
	if err != nil {
		return nil, errors.Wrap(err, "resolve ref_a")
	}

	var commitB *object.Commit
	if refB != "" {
		commitB, err = resolveRefToCommitObject(repo, refB)
		if err != nil {
			return nil, errors.Wrap(err, "resolve ref_b")
		}
	}
	if refB == "" && len(commitA.ParentHashes) > 0 {
		commitB, err = repo.CommitObject(commitA.ParentHashes[0])
		if err != nil {
			return nil, errors.Wrap(err, "parent commit")
		}
	}

	patch, err := commitA.Patch(commitB)
	if err != nil {
		return nil, errors.Wrap(err, "compute patch")
	}
	return patch, nil
}

// resolveRefToCommitObject resolves a ref name to a commit object.
func resolveRefToCommitObject(repo *git.Repository, refName string) (*object.Commit, error) {
	hash, err := resolveRefToCommit(repo, refName)
	if err != nil {
		return nil, err
	}
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return nil, errors.Wrap(err, "commit object")
	}
	return commit, nil
}

// commitToInfo converts a go-git commit to a CommitInfo proto message.
func commitToInfo(c *object.Commit) *s4wave_git.CommitInfo {
	info := &s4wave_git.CommitInfo{
		Hash:            c.Hash.String(),
		Message:         c.Message,
		AuthorName:      c.Author.Name,
		AuthorEmail:     c.Author.Email,
		AuthorTimestamp: c.Author.When.Unix(),
	}
	for _, ph := range c.ParentHashes {
		info.ParentHashes = append(info.ParentHashes, ph.String())
	}
	return info
}

// resolveRefToCommit resolves a revision string to a commit hash.
// Supports full/short hashes, branch/tag names, and rev-parse syntax (HEAD~3, master^2, etc).
// Empty string resolves to HEAD.
func resolveRefToCommit(repo *git.Repository, rev string) (plumbing.Hash, error) {
	if rev == "" {
		rev = "HEAD"
	}
	hash, err := repo.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return plumbing.ZeroHash, errors.Wrap(err, "resolve ref "+rev)
	}
	return *hash, nil
}

// resolveRefToTree resolves a ref name, commit hash, or HEAD (if empty) to a tree hash.
func resolveRefToTree(repo *git.Repository, refName string) (plumbing.Hash, error) {
	commitHash, err := resolveRefToCommit(repo, refName)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	commit, err := repo.CommitObject(commitHash)
	if err != nil {
		return plumbing.ZeroHash, errors.Wrap(err, "commit object")
	}
	return commit.TreeHash, nil
}

// _ is a type assertion
var _ s4wave_git.SRPCGitRepoResourceServiceServer = (*GitRepoResource)(nil)
