//go:build !js

package bldr_plugin_compiler_go

import (
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
)

// addCompilerStartupCacheInputs records the environment-dependent inputs that
// must invalidate the startup cache for the selected compiler.
func addCompilerStartupCacheInputs(
	inputManifest *bldr_manifest_builder.InputManifest,
	goCompilerOpt GoCompiler,
	goCompiler gocompiler.GoCompiler,
) {
	// Record the selected compiler and the inputs that change its output.
	if goCompiler.IsTinyGo() {
		inputManifest.AddEnvStartupInputs(gocompiler.TinyGoStartupCacheEnvKeys())
	}
	if goCompilerOpt == GoCompiler_GO_COMPILER_DEFAULT {
		inputManifest.AddEnvStartupInputs(gocompiler.GoCompilerStartupCacheEnvKeys())
	}
	if goCompiler == gocompiler.GoCompilerGo {
		inputManifest.AddEnvStartupInputs(gocompiler.GoWasmOptimizeStartupCacheEnvKeys())
		inputManifest.AddEnvStartupInputs(gocompiler.GoWasmDiagnosticStartupCacheEnvKeys())

		// Signed and unsigned executables must never share a startup cache entry.
		inputManifest.AddEnvStartupInputs(gocompiler.WindowsSignStartupCacheEnvKeys())
		inputManifest.AddEnvStartupInputs(gocompiler.MacOSSignStartupCacheEnvKeys())
	}
	if goCompiler.IsGoScript() {
		inputManifest.AddEnvStartupInputs(gocompiler.GoScriptStartupCacheEnvKeys())
	}
}
