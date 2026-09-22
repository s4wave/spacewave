//go:build !js

package bldr_plugin_compiler_go

import (
	"testing"

	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
)

// TestStartupSigningIdentity records signing changes for default and explicit Go builds.
func TestStartupSigningIdentity(t *testing.T) {
	// Explicit Go selection must preserve the same signing identity as the default.
	t.Setenv(gocompiler.WindowsSignCommandEnv, "/build/sign")
	t.Setenv(gocompiler.WindowsSignIdentityEnv, "example-product/publisher")
	for _, mode := range []GoCompiler{GoCompiler_GO_COMPILER_DEFAULT, GoCompiler_GO_COMPILER_GO} {
		manifest := bldr_manifest_builder.NewInputManifest(nil, nil)
		addCompilerStartupCacheInputs(manifest, mode, gocompiler.GoCompilerGo)
		values := make(map[string]string)
		for _, input := range manifest.GetStartupInputs() {
			values[input.GetKey()] = input.GetStringValue()
		}
		if values[gocompiler.WindowsSignCommandEnv] != "/build/sign" {
			t.Fatal("signing command was not recorded")
		}
		if values[gocompiler.WindowsSignIdentityEnv] != "example-product/publisher" {
			t.Fatal("signing identity was not recorded")
		}
		if _, present := values[gocompiler.WindowsSignProfileEnv]; !present {
			t.Fatal("unset Azure profile was not recorded")
		}
	}
}
