//go:build tinygo

package space_exec

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	"github.com/sirupsen/logrus"
)

// GitCloneConfigID is the config ID for the space-aware git clone handler.
const GitCloneConfigID = "forge/lib/git/clone"

// NewGitCloneHandler reports that Git clone is unavailable in browser builds.
func NewGitCloneHandler(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	handle forge_target.ExecControllerHandle,
	inputs forge_target.InputMap,
	configData []byte,
) (Handler, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("git clone is not available in this browser build")
}

// RegisterGitClone registers the Git clone handler in the registry.
func RegisterGitClone(r *Registry) {
	r.Register(GitCloneConfigID, NewGitCloneHandler)
}
