package space_exec

import (
	"context"

	"github.com/go-git/go-git/v6/plumbing/client"
	transport_ssh "github.com/go-git/go-git/v6/plumbing/transport/ssh"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/sirupsen/logrus"

	forge_lib_git_clone "github.com/s4wave/spacewave/forge/lib/git/clone"
)

// GitCloneConfigID is the config ID for the space-aware git clone handler.
// Matches the existing forge/lib/git/clone ConfigID so existing task targets work.
var GitCloneConfigID = forge_lib_git_clone.ConfigID

const (
	// outputNameRepo is the name of the output for the repo snapshot.
	outputNameRepo = "repo"
)

// gitCloneHandler executes git clone operations using world state directly.
type gitCloneHandler struct {
	le     *logrus.Entry
	ws     world.WorldState
	handle forge_target.ExecControllerHandle
	conf   *forge_lib_git_clone.Config
}

// Execute runs the git clone handler.
func (h *gitCloneHandler) Execute(ctx context.Context) error {
	sender := h.handle.GetPeerId()
	ts := h.handle.GetTimestamp()
	repoObjKey := h.conf.GetObjectKey()

	alreadyExistsObj, alreadyExists, err := h.ws.GetObject(ctx, repoObjKey)
	if err != nil {
		return err
	}

	cloneOpts := h.conf.GetCloneOpts()
	worktreeOpts := h.conf.GetWorktreeOpts().CloneVT()
	if worktreeOpts != nil {
		worktreeOpts.RepoObjectKey = repoObjKey
		worktreeOpts.Timestamp = ts
	}

	var repoRef *bucket.ObjectRef
	if alreadyExists {
		var repoRev uint64
		repoRef, repoRev, err = alreadyExistsObj.GetRootRef(ctx)
		if err != nil {
			return err
		}
		h.le.Infof("repo already exists at rev %d: %s", repoRev, repoObjKey)

		if repoRev > 1 && !cloneOpts.GetDisableCheckout() && worktreeOpts.GetObjectKey() != "" {
			h.le.Info("initializing worktree from existing repo")
			_, err := worktreeOpts.ApplyWorldOp(ctx, h.le, h.ws, sender)
			if err != nil {
				return err
			}
		}
	} else {
		authMethod, err := resolveAuthWithoutBus(h.conf.GetAuthOpts())
		if err != nil {
			return errors.Wrap(err, "resolve auth")
		}

		h.le.Debugf(
			"git: clone %q to object %q worktree %q",
			cloneOpts.GetUrl(),
			repoObjKey,
			worktreeOpts.GetObjectKey(),
		)
		repoRef, err = git_world.GitClone(
			ctx,
			h.ws,
			repoObjKey,
			sender,
			cloneOpts,
			authMethod,
			nil, // progress
			worktreeOpts,
			ts,
		)
		if err != nil {
			return err
		}
	}

	outps := forge_value.ValueSlice{
		forge_value.NewValueWithBucketRef(outputNameRepo, repoRef),
	}
	return h.handle.SetOutputs(ctx, outps, true)
}

// resolveAuthWithoutBus resolves auth from AuthOpts without bus access.
// Supports username-only SSH auth for public repos. Peer-ID-based private key
// auth requires the caller to provide a pre-resolved key and is not yet wired.
func resolveAuthWithoutBus(a *git_block.AuthOpts) (client.SSHAuth, error) {
	if a == nil {
		return nil, nil
	}
	if a.GetPeerId() != "" {
		return nil, errors.New("peer-ID-based SSH auth not yet supported in space exec handlers")
	}
	username := a.GetUsername()
	if username != "" {
		return &transport_ssh.PublicKeys{User: username}, nil
	}
	return nil, nil
}

// NewGitCloneHandler constructs a git clone space handler.
// Deserializes configData as the forge/lib/git/clone Config proto and executes
// the clone using world state directly (no bus access).
func NewGitCloneHandler(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	handle forge_target.ExecControllerHandle,
	inputs forge_target.InputMap,
	configData []byte,
) (Handler, error) {
	conf := &forge_lib_git_clone.Config{}
	if len(configData) > 0 {
		if err := conf.UnmarshalVT(configData); err != nil {
			return nil, errors.Wrap(err, "unmarshal git clone config")
		}
	}
	if err := conf.Validate(); err != nil {
		return nil, errors.Wrap(err, "validate git clone config")
	}

	return &gitCloneHandler{
		le:     le,
		ws:     ws,
		handle: handle,
		conf:   conf,
	}, nil
}

// RegisterGitClone registers the git clone handler in the registry.
func RegisterGitClone(r *Registry) {
	r.Register(GitCloneConfigID, NewGitCloneHandler)
}

// _ is a type assertion
var _ Handler = (*gitCloneHandler)(nil)
