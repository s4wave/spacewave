//go:build !tinygo && !goscript

package optypes

import "github.com/aperturerobotics/common/typeid"

// gitBlockTypes is empty in the native build, where
// core/space/world/optypes/git registers the Git types.
var gitBlockTypes []typeid.Type
