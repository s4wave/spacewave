//go:build !tinygo

package space_exec

import (
	"context"

	"github.com/go-git/go-git/v6/plumbing/client"
	transport_ssh "github.com/go-git/go-git/v6/plumbing/transport/ssh"
	"github.com/pkg/errors"
	git_block "github.com/s4wave/spacewave/db/git/block"
	"github.com/s4wave/spacewave/db/world"
	forge_lib_git_clone "github.com/s4wave/spacewave/forge/lib/git/clone"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/sirupsen/logrus"
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
	// le is the logger.
	le *logrus.Entry
	// ws is the Space World that holds the repository.
	ws world.WorldState
	// handle supplies the sender and timestamp and receives the outputs.
	handle forge_target.ExecControllerHandle
	// conf is the validated clone configuration.
	conf *forge_lib_git_clone.Config
}

// Execute clones the repository, or fetches its branch when the World holds it.
func (h *gitCloneHandler) Execute(ctx context.Context) error {
	// Resolve auth without bus access.
	authMethod, err := resolveAuthWithoutBus(h.conf.GetAuthOpts())
	if err != nil {
		return errors.Wrap(err, "resolve auth")
	}

	// Clone or fetch the repository in the Space World.
	sender, ts := h.handle.GetPeerId(), h.handle.GetTimestamp()
	repoRef, err := h.conf.CloneOrFetch(ctx, h.le, h.ws, sender, ts, authMethod, nil)
	if err != nil {
		return err
	}

	// Publish the repo snapshot reference as the handler output.
	outps := forge_value.ValueSlice{
		forge_value.NewValueWithBucketRef(outputNameRepo, repoRef),
	}
	return h.handle.SetOutputs(ctx, outps, true)
}

// resolveAuthWithoutBus resolves auth from AuthOpts without bus access.
// Supports username-only SSH auth for public repos. Peer-ID-based private key
// auth requires the caller to provide a pre-resolved key and is not yet wired.
func resolveAuthWithoutBus(a *git_block.AuthOpts) (client.SSHAuth, error) {
	// Reject peer-ID auth and fall back to username-only or anonymous SSH auth.
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
