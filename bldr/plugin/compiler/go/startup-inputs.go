//go:build !js

package bldr_plugin_compiler_go

import (
	"os"

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
		addTinyGoStartupCacheInputs(inputManifest)
	}
	if goCompilerOpt == GoCompiler_GO_COMPILER_DEFAULT {
		addGoCompilerStartupCacheInputs(inputManifest)
	}
	if goCompiler == gocompiler.GoCompilerGo {
		addGoWasmOptimizeStartupCacheInputs(inputManifest)
		addGoWasmDiagnosticStartupCacheInputs(inputManifest)

		// Signed and unsigned executables must never share a startup cache entry.
		for _, key := range gocompiler.WindowsSignStartupCacheEnvKeys() {
			inputManifest.AddStartupInput(bldr_manifest_builder.NewEnvStartupInput(key, os.Getenv(key)))
		}
		inputManifest.SortStartupInputs()
	}
	if goCompiler.IsGoScript() {
		addGoScriptStartupCacheInputs(inputManifest)
	}
}

// addTinyGoStartupCacheInputs adds tinygo env cache inputs to the manifest.
func addTinyGoStartupCacheInputs(inputManifest *bldr_manifest_builder.InputManifest) {
	for _, envKey := range gocompiler.TinyGoStartupCacheEnvKeys() {
		inputManifest.AddStartupInput(bldr_manifest_builder.NewEnvStartupInput(envKey, os.Getenv(envKey)))
	}
	inputManifest.SortStartupInputs()
}

// addGoCompilerStartupCacheInputs adds default go compiler env cache inputs.
func addGoCompilerStartupCacheInputs(inputManifest *bldr_manifest_builder.InputManifest) {
	for _, envKey := range gocompiler.GoCompilerStartupCacheEnvKeys() {
		inputManifest.AddStartupInput(bldr_manifest_builder.NewEnvStartupInput(envKey, os.Getenv(envKey)))
	}
	inputManifest.SortStartupInputs()
}

// addGoWasmOptimizeStartupCacheInputs adds go wasm optimize env cache inputs.
func addGoWasmOptimizeStartupCacheInputs(inputManifest *bldr_manifest_builder.InputManifest) {
	for _, envKey := range gocompiler.GoWasmOptimizeStartupCacheEnvKeys() {
		inputManifest.AddStartupInput(bldr_manifest_builder.NewEnvStartupInput(envKey, os.Getenv(envKey)))
	}
	inputManifest.SortStartupInputs()
}

// addGoWasmDiagnosticStartupCacheInputs adds go wasm diagnostic env cache inputs.
func addGoWasmDiagnosticStartupCacheInputs(inputManifest *bldr_manifest_builder.InputManifest) {
	for _, envKey := range gocompiler.GoWasmDiagnosticStartupCacheEnvKeys() {
		inputManifest.AddStartupInput(bldr_manifest_builder.NewEnvStartupInput(envKey, os.Getenv(envKey)))
	}
	inputManifest.SortStartupInputs()
}

// addGoScriptStartupCacheInputs adds goscript env cache inputs to the manifest.
func addGoScriptStartupCacheInputs(inputManifest *bldr_manifest_builder.InputManifest) {
	for _, envKey := range gocompiler.GoScriptStartupCacheEnvKeys() {
		inputManifest.AddStartupInput(bldr_manifest_builder.NewEnvStartupInput(envKey, os.Getenv(envKey)))
	}
	inputManifest.SortStartupInputs()
}
