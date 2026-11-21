//go:build !tinygo && !goscript

package space_world_ops

import identity_world "github.com/s4wave/spacewave/identity/world"

// buildTypes lists the types of the native build.
//
// The native build leaves the Git operations to
// core/space/world/optypes/git, so a program without Git objects links no
// Git code.
var buildTypes = identity_world.WorldTypes
