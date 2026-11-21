//go:build tinygo || goscript

package optypes

import git_block "github.com/s4wave/spacewave/db/git/block"

// gitBlockTypes lists the Git block types of the TinyGo and GoScript builds.
var gitBlockTypes = git_block.GitTypes
