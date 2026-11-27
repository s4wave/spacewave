package chrometest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
	"github.com/s4wave/spacewave/testbed/reporoot"
)

// buildAssets builds the worker program into the disposable asset directory.
func buildAssets(dir string) error {
	// Build and publish the worker program and browser assets.
	if err := buildWasm(filepath.Join(dir, "testprog.wasm")); err != nil {
		return err
	}
	wasmExec, err := wasmExecPath()
	if err != nil {
		return err
	}
	if err := copyFile(wasmExec, filepath.Join(dir, "wasm_exec.js")); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(indexHTML), 0o644); err != nil {
		return errors.Wrap(err, "write index")
	}
	if err := os.WriteFile(filepath.Join(dir, "worker.js"), []byte(workerJS), 0o644); err != nil {
		return errors.Wrap(err, "write worker")
	}
	return nil
}

// buildWasm compiles the selected worker with the requested Go or TinyGo toolchain.
func buildWasm(out string) error {
	// Resolve the module root within the worker compiler deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	root, err := reporoot.Find()
	if err != nil {
		return err
	}

	// Build the worker program.
	program := "./e2e/opfschrome/testprog"

	// Compile with TinyGo when the worker scenario requests it.
	if os.Getenv(tinyGoEnv) == "1" || strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		// Resolve and report the TinyGo worker compiler arguments.
		envelope, err := resolveTinyGoBuildEnvelope()
		if err != nil {
			return err
		}
		args := append([]string{"build"}, envelope.args()...)
		args = append(args, "-o", out, program)
		os.Stderr.WriteString("opfs chrometest wasm build: compiler=tinygo version=" +
			strconv.Quote(tinyGoVersion(ctx)) + " envelope=" + envelope.String() +
			" args=" + strings.Join(args, " ") + "\n")

		// Compile the TinyGo worker within the build deadline.
		start := time.Now()
		cmd := exec.CommandContext(ctx, "tinygo", args...)
		cmd.Dir = root
		data, err := cmd.CombinedOutput()
		if err != nil {
			return errors.Errorf("tinygo build js/wasm failed: %v\n%s", err, data)
		}

		// Inspect the TinyGo artifact and report its timing and features.
		info, err := os.Stat(out)
		if err != nil {
			return errors.Wrap(err, "stat TinyGo wasm artifact")
		}
		os.Stderr.WriteString("opfs chrometest wasm build result: compiler=tinygo elapsed=" +
			time.Since(start).Round(time.Millisecond).String() + " artifact_bytes=" +
			strconv.FormatInt(info.Size(), 10) + " wasm_features=" + wasmFeatureEvidence(ctx, out) + "\n")
		return nil
	}

	// Compile the worker with the standard Go WebAssembly target.
	cmd := exec.CommandContext(ctx, "go", "build", "-o", out, program)
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Dir = root
	data, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Errorf("go build js/wasm failed: %v\n%s", err, data)
	}
	return nil
}

// tinyGoBuildEnvelope records the exact TinyGo target and compiler settings.
type tinyGoBuildEnvelope struct {
	// profile selects the compiler default policy.
	profile string
	// target selects the TinyGo build target.
	target string
	// opt overrides optimization when nonempty.
	opt string
	// panicStrategy selects panic reporting.
	panicStrategy string
	// gc overrides garbage collection when nonempty.
	gc string
	// scheduler selects goroutine scheduling.
	scheduler string
	// stackSize overrides stack size when nonempty.
	stackSize string
	// llvmFeatures overrides target features when nonempty.
	llvmFeatures string
}

// resolveTinyGoBuildEnvelope validates the profile and resolves requested compiler settings.
func resolveTinyGoBuildEnvelope() (*tinyGoBuildEnvelope, error) {
	// Resolve and validate the requested TinyGo compiler profile.
	profile := strings.TrimSpace(os.Getenv(tinyGoProfileEnv))
	if profile == "" {
		profile = tinyGoProfileCustom
	}
	if profile != tinyGoProfileCustom && profile != tinyGoTargetDefault && profile != tinyGoBldrFeatures {
		return nil, errors.Errorf("unsupported %s=%q, expected custom, target-default, or bldr-features", tinyGoProfileEnv, profile)
	}

	// Apply compiler overrides and defaults for the selected TinyGo profile.
	env := &tinyGoBuildEnvelope{
		profile:       profile,
		target:        "wasm",
		opt:           strings.TrimSpace(os.Getenv(tinyGoOptEnv)),
		panicStrategy: strings.TrimSpace(os.Getenv(tinyGoPanicEnv)),
		gc:            strings.TrimSpace(os.Getenv(tinyGoGCEnv)),
		scheduler:     strings.TrimSpace(os.Getenv(tinyGoSchedulerEnv)),
		stackSize:     strings.TrimSpace(os.Getenv(tinyGoStackEnv)),
		llvmFeatures:  strings.TrimSpace(os.Getenv(tinyGoLLVMEnv)),
	}
	if env.panicStrategy == "" {
		env.panicStrategy = "print"
	}
	if env.scheduler == "" {
		env.scheduler = "asyncify"
	}
	if profile == tinyGoTargetDefault || profile == tinyGoBldrFeatures {
		if env.stackSize == "" {
			env.stackSize = gocompiler.TinyGoDefaultStackSize
		}
	}
	if profile == tinyGoBldrFeatures && env.llvmFeatures == "" {
		env.llvmFeatures = strings.Join(gocompiler.GetDefaultTinygoLlvmFeatures(), ",")
	}
	return env, nil
}

// args returns explicit compiler arguments, preserving target defaults when unset.
func (e *tinyGoBuildEnvelope) args() []string {
	// Encode explicit TinyGo compiler overrides as command arguments.
	args := []string{"-target", e.target}
	if e.opt != "" {
		args = append(args, "-opt="+e.opt)
	}
	if e.scheduler != "" {
		args = append(args, "-scheduler="+e.scheduler)
	}
	if e.panicStrategy != "" {
		args = append(args, "-panic="+e.panicStrategy)
	}
	if e.gc != "" {
		args = append(args, "-gc="+e.gc)
	}
	if e.stackSize != "" {
		args = append(args, "-stack-size="+e.stackSize)
	}
	if e.llvmFeatures != "" {
		args = append(args, "-llvm-features="+e.llvmFeatures)
	}

	return args
}

// String renders the selected compiler envelope for reproducible probe output.
func (e *tinyGoBuildEnvelope) String() string {
	// Resolve target feature and garbage collector defaults for diagnostics.
	features := e.llvmFeatures
	if features == "" {
		features = "target-default"
	}
	gc := e.gc
	if gc == "" {
		gc = "target-default"
	}

	// Resolve optimization and stack defaults for diagnostics.
	opt := e.opt
	if opt == "" {
		opt = "target-default"
	}
	stack := e.stackSize
	if stack == "" {
		stack = "target-default"
	}

	return strings.Join([]string{
		tinyGoProfileEnv + "=" + e.profile,
		tinyGoOptEnv + "=" + opt,
		tinyGoPanicEnv + "=" + e.panicStrategy,
		tinyGoGCEnv + "=" + gc,
		tinyGoSchedulerEnv + "=" + e.scheduler,
		tinyGoStackEnv + "=" + stack,
		tinyGoLLVMEnv + "=" + features,
		"target=" + e.target,
		"bldr_concepts=" + strings.Join([]string{
			gocompiler.TinyGoProfileEnv,
			gocompiler.TinyGoOptEnv,
			gocompiler.TinyGoPanicStrategyEnv,
			gocompiler.TinyGoGCEnv,
			gocompiler.TinyGoSchedulerEnv,
			gocompiler.TinyGoStackSizeEnv,
			gocompiler.TinyGoLLVMFeaturesEnv,
		}, ","),
	}, " ")
}

// tinyGoVersion reads the compiler version within a bounded subprocess lifetime.
func tinyGoVersion(ctx context.Context) string {
	// Read the TinyGo compiler version within a bounded subprocess.
	versionCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(versionCtx, "tinygo", "version")
	data, err := cmd.CombinedOutput()
	if err != nil {
		return err.Error()
	}

	return strings.TrimSpace(string(data))
}

// wasmFeatureEvidence summarizes available feature evidence from the compiled artifact.
func wasmFeatureEvidence(ctx context.Context, wasmPath string) string {
	// Inspect the WebAssembly artifact within a bounded subprocess.
	objdumpCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(objdumpCtx, "wasm-objdump", "-x", wasmPath)
	data, err := cmd.CombinedOutput()
	if err != nil {
		return "wasm-objdump-error=" + err.Error()
	}

	// Collect enabled and disabled target features from the artifact dump.
	var features []string
	inTargetFeatures := false
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, `name: "target_features"`) {
			inTargetFeatures = true
			continue
		}
		if !inTargetFeatures {
			continue
		}
		if strings.HasPrefix(line, "- [+]") || strings.HasPrefix(line, "- [-]") {
			features = append(features, strings.Join(strings.Fields(line), " "))
			continue
		}
		if len(features) != 0 && line != "" {
			break
		}
	}
	if len(features) == 0 {
		return "target_features=none"
	}

	return strings.Join(features, ";")
}

// wasmExecPath finds the runtime glue corresponding to the selected compiler.
func wasmExecPath() (string, error) {
	// Find runtime glue for the selected TinyGo compiler.
	if os.Getenv(tinyGoEnv) == "1" || strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "tinygo", "env", "TINYGOROOT")
		data, err := cmd.Output()
		if err != nil {
			return "", errors.Wrap(err, "tinygo env TINYGOROOT")
		}

		// Validate the compiler root before selecting its runtime glue.
		tinyGoRoot := strings.TrimSpace(string(data))
		if tinyGoRoot == "" {
			return "", errors.New("tinygo env TINYGOROOT returned empty path")
		}

		return filepath.Join(tinyGoRoot, "targets", "wasm_exec.js"), nil
	}

	// Find standard Go runtime glue when TinyGo is not selected.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "env", "GOROOT")
	data, err := cmd.Output()
	if err != nil {
		return "", errors.Wrap(err, "go env GOROOT")
	}
	goroot := strings.TrimSpace(string(data))
	if goroot == "" {
		return "", errors.New("go env GOROOT returned empty path")
	}

	return filepath.Join(goroot, "lib", "wasm", "wasm_exec.js"), nil
}

// copyFile copies one generated asset into the test server directory.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return errors.Wrap(err, "read "+src)
	}
	return os.WriteFile(dst, data, 0o644)
}
