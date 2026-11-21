//go:build !tinygo && !goscript

package optypes

import git_world "github.com/s4wave/spacewave/db/git/world"

// The native build takes the Git types only in a program that imports
// core/space/world/optypes/git, which this package cannot import. Register the
// Git operations here so the tests cover them as they do in such a program and
// in the GoScript build, whose schema always holds them.
func init() {
	Register(git_world.GitTypes)
}
