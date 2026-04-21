// package bldr_values contains value types that are injected to the target binary.
package bldr_values

import bldr_web_bundler "github.com/s4wave/spacewave/bldr/web/bundler"

// VoidOutput contains no output.
type VoidOutput = struct{}

// WebBundlerOutput contains a single web bundler output object.
// EsbuildVarType_WEB_BUNDLER_OUTPUT
// ViteVarType_WEB_BUNDLER_OUTPUT
type WebBundlerOutput = bldr_web_bundler.WebBundlerOutput
