//go:build !tinygo && !goscript

package optypes_git

import (
	s4wave_git "github.com/s4wave/spacewave/core/git"
	"github.com/s4wave/spacewave/core/space/world/optypes"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
)

func init() {
	optypes.Register(git_world.GitTypes, s4wave_git.GitTypes, git_block.GitTypes)
}
