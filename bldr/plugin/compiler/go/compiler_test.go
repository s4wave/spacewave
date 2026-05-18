//go:build !js

package bldr_plugin_compiler_go

import (
	"testing"

	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
)

func TestAddTinyGoStartupCacheInputsIncludesProfileIdentity(t *testing.T) {
	values := map[string]string{
		gocompiler.TinyGoProfileEnv:       gocompiler.TinyGoProfileFast,
		gocompiler.TinyGoOptEnv:           "0",
		gocompiler.TinyGoPanicStrategyEnv: "print",
		gocompiler.TinyGoGCEnv:            "leaking",
		gocompiler.TinyGoSchedulerEnv:     "none",
		gocompiler.TinyGoLLVMFeaturesEnv:  "+atomics,+bulk-memory",
		gocompiler.TinyGoInterpTimeoutEnv: "10m",
	}
	for _, key := range gocompiler.TinyGoStartupCacheEnvKeys() {
		t.Setenv(key, values[key])
	}

	inputManifest := bldr_manifest_builder.NewInputManifest(nil, nil)
	addTinyGoStartupCacheInputs(inputManifest)

	got := make(map[string]string, len(inputManifest.GetStartupInputs()))
	for _, input := range inputManifest.GetStartupInputs() {
		if input.GetKind() != bldr_manifest_builder.InputManifest_StartupInputKind_ENV_VAR {
			t.Fatalf("startup input kind = %v, want env var", input.GetKind())
		}
		got[input.GetKey()] = input.GetStringValue()
	}
	for _, key := range gocompiler.TinyGoStartupCacheEnvKeys() {
		if got[key] != values[key] {
			t.Fatalf("startup input %s = %q, want %q", key, got[key], values[key])
		}
	}
}
