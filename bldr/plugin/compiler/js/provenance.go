//go:build !js

package bldr_plugin_compiler_js

import (
	"os"
	"path/filepath"

	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
)

// PreBuildHookProvenance declares the complete deterministic provenance of a
// pre-build hook. A hook that declares provenance promises its outputs are a
// pure function of the declared input files and environment variables, together
// with the builder controller config the startup cache already digests.
// Declaring provenance makes the hook eligible for startup cache reuse; a hook
// registered without a declaration keeps the compiler always-building. The
// declaration describes inputs only and never runtime behavior.
type PreBuildHookProvenance struct {
	// InputFiles are exact input file paths, relative to the builder source
	// path, that fully determine the hook's outputs. Each is validated by
	// content identity on startup; a change or removal forces a rebuild.
	InputFiles []string
	// EnvVars are the environment variable names the hook's outputs depend on.
	// Each is validated on startup; a changed value forces a rebuild.
	EnvVars []string
}

// StartupInputPaths resolves the declared input files to absolute paths under
// sourcePath for folding into the build's validated startup input set.
func (p *PreBuildHookProvenance) StartupInputPaths(sourcePath string) []string {
	if p == nil {
		return nil
	}
	paths := make([]string, 0, len(p.InputFiles))
	for _, inputFile := range p.InputFiles {
		if inputFile == "" {
			continue
		}
		filePath := inputFile
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(sourcePath, filepath.FromSlash(inputFile))
		}
		paths = append(paths, filePath)
	}
	return paths
}

// EnvStartupInputs builds validated environment startup inputs for the declared
// environment variables, capturing their current values.
func (p *PreBuildHookProvenance) EnvStartupInputs() []*bldr_manifest_builder.InputManifest_StartupInput {
	if p == nil {
		return nil
	}
	inputs := make([]*bldr_manifest_builder.InputManifest_StartupInput, 0, len(p.EnvVars))
	for _, envKey := range p.EnvVars {
		if envKey == "" {
			continue
		}
		inputs = append(inputs, bldr_manifest_builder.NewEnvStartupInput(envKey, os.Getenv(envKey)))
	}
	return inputs
}
