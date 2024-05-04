package opt_wasm

import (
	"os"
	"path/filepath"

	uexec "github.com/aperturerobotics/util/exec"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/sirupsen/logrus"
)

// OptimizeWasmBinary optimizes a .wasm binary using wasm-opt.
func OptimizeWasmBinary(le *logrus.Entry, workingPath, outBinPath string) error {
	// track file size savings
	preOptStat, err := os.Stat(outBinPath)
	if err != nil {
		return err
	}
	preOptSize := preOptStat.Size()

	// wasm-opt
	// wasm-opt -Oz -o ./out.wasm.opt ./out.wasm
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
	ecmd := uexec.NewCmd("wasm-opt", "--enable-bulk-memory", "-Oz", "-o", optPathRel, outBinPathRel)
	ecmd.Env = os.Environ()
	ecmd.Dir = workingPath
	if err := uexec.ExecCmd(le, ecmd); err != nil {
		return err
	}
	if err := fsutil.MoveFile(outBinPath, optPath, 0o644); err != nil {
		return err
	}

	postOptStat, err := os.Stat(outBinPath)
	if err != nil {
		return err
	}
	postOptSize := postOptStat.Size()

	le.Infof("optimized %s from %d -> %d bytes delta %d", outBinFilename, preOptSize, postOptSize, postOptSize-preOptSize)
	return nil
}
