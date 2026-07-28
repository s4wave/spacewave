//go:build !js

package releasewasm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
	"github.com/s4wave/spacewave/e2e/releasewasm/artifact"
	"github.com/sirupsen/logrus"
)

const releaseWasmArtifactStoreRelPath = ".bldr/e2e-releasewasm/release-artifacts"

var releaseWasmArtifactEnvKeys = []string{
	E2EReleaseWasmTinyGoEnv,
	E2EReleaseWasmGoScriptEnv,
	E2EReleaseWasmTinyGoProfileEnv,
	E2EReleaseWasmTinyGoOptEnv,
	E2EReleaseWasmTinyGoPanicEnv,
	E2EReleaseWasmTinyGoGCEnv,
	E2EReleaseWasmTinyGoSchedulerEnv,
	E2EReleaseWasmTinyGoStackSizeEnv,
	E2EReleaseWasmTinyGoLLVMFeaturesEnv,
	E2EReleaseWasmTinyGoInterpTimeoutEnv,
	E2EReleaseWasmTinyGoDebugInfoEnv,
	gocompiler.TinyGoProfileEnv,
	gocompiler.TinyGoOptEnv,
	gocompiler.TinyGoPanicStrategyEnv,
	gocompiler.TinyGoGCEnv,
	gocompiler.TinyGoSchedulerEnv,
	gocompiler.TinyGoStackSizeEnv,
	gocompiler.TinyGoLLVMFeaturesEnv,
	gocompiler.TinyGoInterpTimeoutEnv,
	gocompiler.TinyGoDebugInfoEnv,
	gocompiler.GoWasmOptimizeEnv,
	gocompiler.RuntimeStartupTraceEnv,
	"BINARYEN_CORES",
	"BINARYEN_VERSION",
	"CGO_ENABLED",
	"GOEXPERIMENT",
	"GOFLAGS",
	"GOTOOLCHAIN",
	"TINYGO_VERSION",
}

// PublishReleaseWasmArtifact publishes the existing release and prerender build
// outputs under their current source and build identity.
func PublishReleaseWasmArtifact(ctx context.Context, le *logrus.Entry, repoRoot string) error {
	identity, err := computeReleaseWasmArtifactIdentity(ctx, repoRoot)
	if err != nil {
		return err
	}
	_, _, err = artifact.Publish(
		releaseWasmArtifactStoreDir(repoRoot),
		filepath.Join(repoRoot, releaseDistRelPath),
		filepath.Join(repoRoot, prerenderDistRelPath),
		identity,
	)
	if err != nil {
		return err
	}
	// The consumer of this artifact computes the same identity in its own
	// process, and when the two disagree it silently rebuilds. Print the
	// producing side's digests so the pair can be compared from two job logs.
	le.WithFields(identityFields(identity)).Info("published release artifact")
	return nil
}

func computeReleaseWasmArtifactIdentity(ctx context.Context, repoRoot string) (*artifact.Identity, error) {
	compiler, err := resolveReleaseWasmCompiler()
	if err != nil {
		return nil, err
	}
	if compiler == releaseWasmCompilerTinyGo {
		if err := applyReleaseWasmTinyGoCompilerEnv(); err != nil {
			return nil, errors.Wrap(err, "apply release wasm TinyGo compiler env")
		}
	}

	environment := make(map[string]string, len(releaseWasmArtifactEnvKeys)+1)
	for _, key := range releaseWasmArtifactEnvKeys {
		environment[key] = strings.TrimSpace(os.Getenv(key))
	}
	environment["E2E_RELEASE_WASM_BUILD_SCRIPT"] = releaseWasmBuildScript()

	tools := make(map[string]string, 3)
	tools["go"], err = releaseWasmToolVersion(ctx, repoRoot, "go", "version")
	if err != nil {
		return nil, err
	}
	tools["bun"], err = releaseWasmToolVersion(ctx, repoRoot, "bun", "--version")
	if err != nil {
		return nil, err
	}
	if compiler == releaseWasmCompilerTinyGo {
		tools["tinygo"] = environment["TINYGO_VERSION"]
		if tools["tinygo"] == "" {
			tools["tinygo"], err = releaseWasmToolVersion(ctx, repoRoot, "tinygo", "version")
			if err != nil {
				return nil, err
			}
		}
	}
	_, wasmOptErr := exec.LookPath("wasm-opt")
	switch {
	case environment["BINARYEN_VERSION"] != "":
		tools["wasm-opt"] = environment["BINARYEN_VERSION"]
	case wasmOptErr == nil:
		tools["wasm-opt"], err = releaseWasmToolVersion(ctx, repoRoot, "wasm-opt", "--version")
		if err != nil {
			return nil, err
		}
	default:
		tools["wasm-opt"] = "absent"
	}

	return artifact.ComputeIdentity(repoRoot, &artifact.BuildInputs{
		Compiler:    string(compiler),
		Mode:        "release/web/e2e",
		Environment: environment,
		Tools:       tools,
	})
}

func releaseWasmToolVersion(ctx context.Context, repoRoot, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrapf(err, "read %s version", name)
	}
	return strings.TrimSpace(string(output)), nil
}

func releaseWasmArtifactStoreDir(repoRoot string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(releaseWasmArtifactStoreRelPath))
}
