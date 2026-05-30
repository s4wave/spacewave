package opt_wasm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	uexec "github.com/aperturerobotics/util/exec"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/sirupsen/logrus"
)

const wasmOptDiagnosticsDirEnv = "BLDR_WASM_OPT_DIAGNOSTICS_DIR"

// OptimizeWasmBinary optimizes a .wasm binary using wasm-opt.
func OptimizeWasmBinary(ctx context.Context, le *logrus.Entry, workingPath, outBinPath string) error {
	// track file size savings
	preOptStat, err := os.Stat(outBinPath)
	if err != nil {
		return err
	}
	preOptSize := preOptStat.Size()

	outBinDir, outBinFilename := filepath.Dir(outBinPath), filepath.Base(outBinPath)
	optFilename := outBinFilename + ".wasm-opt"

	outBinDirRel, err := filepath.Rel(workingPath, outBinDir)
	if err != nil {
		return err
	}

	outBinPathRel, err := filepath.Rel(workingPath, outBinPath)
	if err != nil {
		return err
	}

	optPathRel := filepath.Join(outBinDirRel, optFilename)
	optPath := filepath.Join(workingPath, optPathRel)

	// -Os: optimized .wasm binary from 34580687 -> 32068818 bytes delta -2511869
	// -Oz: optimized .wasm binary from 34580687 -> 29498128 bytes delta -5082559
	args := []string{
		// https://caniuse.com/?search=WebAssembly
		// Baseline 2023: https://caniuse.com/wasm-simd
		"--enable-simd",
		// All browsers support: https://caniuse.com/wasm-signext
		"--enable-sign-ext",
		// All browsers support: https://caniuse.com/wasm-threads
		"--enable-threads",
		// All browsers support: https://caniuse.com/wasm-bulk-memory
		// Required by: go
		"--enable-bulk-memory",
		// All browsers support: https://caniuse.com/wasm-multi-value
		"--enable-multivalue",
		// All browsers support: https://caniuse.com/wasm-mutable-globals
		"--enable-mutable-globals",
		// All browsers support: https://caniuse.com/wasm-reference-types
		"--enable-reference-types",
		// All browsers support: https://caniuse.com/wasm-nontrapping-fptoint
		"--enable-nontrapping-float-to-int",

		// Optimize for size (z is even smaller)
		"-Os", // "-Oz",

		"-o", optPathRel,
		outBinPathRel,
	}
	timeStart := time.Now()
	if err := runWasmOpt(ctx, le, "optimize", workingPath, outBinPath, outBinPathRel, optPathRel, args); err != nil {
		return err
	}
	if err := fsutil.MoveFile(outBinPath, optPath, 0o644); err != nil {
		return err
	}
	dur := time.Since(timeStart)

	postOptStat, err := os.Stat(outBinPath)
	if err != nil {
		return err
	}
	postOptSize := postOptStat.Size()

	le.
		WithField("dur", dur.String()).
		Infof("optimized %s from %d -> %d bytes delta %d", outBinFilename, preOptSize, postOptSize, postOptSize-preOptSize)
	return nil
}

// StripWasmDebugSections removes debug custom sections without optimization.
func StripWasmDebugSections(ctx context.Context, le *logrus.Entry, workingPath, outBinPath string) error {
	preOptStat, err := os.Stat(outBinPath)
	if err != nil {
		return err
	}
	preOptSize := preOptStat.Size()

	outBinDir, outBinFilename := filepath.Dir(outBinPath), filepath.Base(outBinPath)
	optFilename := outBinFilename + ".wasm-strip"

	outBinDirRel, err := filepath.Rel(workingPath, outBinDir)
	if err != nil {
		return err
	}

	outBinPathRel, err := filepath.Rel(workingPath, outBinPath)
	if err != nil {
		return err
	}

	optPathRel := filepath.Join(outBinDirRel, optFilename)
	optPath := filepath.Join(workingPath, optPathRel)

	args := []string{
		"--enable-simd",
		"--enable-sign-ext",
		"--enable-threads",
		"--enable-bulk-memory",
		"--enable-multivalue",
		"--enable-mutable-globals",
		"--enable-reference-types",
		"--enable-nontrapping-float-to-int",

		"--strip-debug",
		"--strip-dwarf",

		"-o", optPathRel,
		outBinPathRel,
	}
	timeStart := time.Now()
	if err := runWasmOpt(ctx, le, "strip-debug", workingPath, outBinPath, outBinPathRel, optPathRel, args); err != nil {
		return err
	}
	if err := fsutil.MoveFile(outBinPath, optPath, 0o644); err != nil {
		return err
	}
	dur := time.Since(timeStart)

	postOptStat, err := os.Stat(outBinPath)
	if err != nil {
		return err
	}
	postOptSize := postOptStat.Size()

	le.
		WithField("dur", dur.String()).
		Infof("stripped debug sections from %s from %d -> %d bytes delta %d", outBinFilename, preOptSize, postOptSize, postOptSize-preOptSize)
	return nil
}

func runWasmOpt(
	ctx context.Context,
	le *logrus.Entry,
	mode,
	workingPath,
	outBinPath,
	outBinPathRel,
	optPathRel string,
	args []string,
) error {
	inputSize, inputHash, err := wasmInputEvidence(outBinPath)
	if err != nil {
		return err
	}
	version := wasmOptVersion(ctx)
	argv := append([]string{"wasm-opt"}, args...)
	diagnosticsDir := wasmOptDiagnosticsDir()
	diagnosticsPreserved := false

	le.WithFields(logrus.Fields{
		"mode":             mode,
		"input":            outBinPathRel,
		"input-bytes":      inputSize,
		"input-sha256":     inputHash,
		"output":           optPathRel,
		"wasm-opt-version": version,
		"argv":             strings.Join(argv, " "),
	}).Info("running wasm-opt")

	if diagnosticsDir != "" {
		if diagErr := preserveWasmOptDiagnostics(diagnosticsDir, mode, outBinPath, outBinPathRel, optPathRel, inputSize, inputHash, version, argv); diagErr != nil {
			le.WithError(diagErr).Warn("failed to preserve wasm-opt diagnostics before execution")
		} else {
			diagnosticsPreserved = true
		}
	}

	ecmd := uexec.NewCmd(ctx, "wasm-opt", args...)
	ecmd.Env = os.Environ()
	ecmd.Dir = workingPath
	if err := uexec.ExecCmd(le, ecmd); err != nil {
		if diagnosticsDir != "" && !diagnosticsPreserved {
			if diagErr := preserveWasmOptDiagnostics(diagnosticsDir, mode, outBinPath, outBinPathRel, optPathRel, inputSize, inputHash, version, argv); diagErr != nil {
				le.WithError(diagErr).Warn("failed to preserve wasm-opt diagnostics")
			}
		}
		return err
	}
	return nil
}

func wasmOptDiagnosticsDir() string {
	return strings.TrimSpace(os.Getenv(wasmOptDiagnosticsDirEnv))
}

func wasmInputEvidence(path string) (int64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0, "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return 0, "", err
	}
	return info.Size(), hex.EncodeToString(h.Sum(nil)), nil
}

func wasmOptVersion(ctx context.Context) string {
	versionCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(versionCtx, "wasm-opt", "--version")
	data, err := cmd.CombinedOutput()
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(data))
}

func preserveWasmOptDiagnostics(
	dir,
	mode,
	outBinPath,
	outBinPathRel,
	optPathRel string,
	inputSize int64,
	inputHash,
	version string,
	argv []string,
) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	hashPrefix := inputHash
	if len(hashPrefix) > 16 {
		hashPrefix = hashPrefix[:16]
	}
	base := fmt.Sprintf("%s.%s.%s", filepath.Base(outBinPath), mode, hashPrefix)
	wasmPath := filepath.Join(dir, base+".input.wasm")
	metaPath := filepath.Join(dir, base+".wasm-opt.txt")
	if err := copyFile(wasmPath, outBinPath); err != nil {
		return err
	}
	meta := fmt.Sprintf(
		"mode: %s\ninput: %s\noutput: %s\ninput_bytes: %d\ninput_sha256: %s\nwasm_opt_version: %s\nargv: %s\n",
		mode,
		outBinPathRel,
		optPathRel,
		inputSize,
		inputHash,
		version,
		strings.Join(argv, " "),
	)
	return os.WriteFile(metaPath, []byte(meta), 0o644)
}

func copyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
