//go:build !tinygo && !goscript

package optypes

import git_world "github.com/s4wave/spacewave/db/git/world"

// init registers the Git operations here so the tests cover them as in a
// native program that imports core/space/world/optypes/git and in the GoScript
// build, whose schema always holds them. The native build takes the Git types
// only in a program with that import, which this package cannot use.
func init() {
	Register(git_world.GitTypes)
}
