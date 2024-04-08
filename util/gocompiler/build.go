package gocompiler

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	bldr_platform_go "github.com/aperturerobotics/bldr/platform/go"
	uexec "github.com/aperturerobotics/util/exec"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/sirupsen/logrus"
)

// ExecBuildEntrypoint executes building an entrypoint main package.
func ExecBuildEntrypoint(
	le *logrus.Entry,
	buildPlatform bldr_platform.Platform,
	buildType bldr_manifest.BuildType,
	workingPath,
	outBinPath string,
	enableCgo bool,
	buildTags []string,
	ldFlags []string,
) error {
	isRelease := buildType.IsRelease()
	isNativeBuildPlatform := buildPlatform.GetBasePlatformID() == bldr_platform.PlatformID_NATIVE
	isWebBuildPlatform := buildPlatform.GetBasePlatformID() == bldr_platform.PlatformID_WEB

	platformEnv, err := bldr_platform_go.PlatformToGoEnv(buildPlatform)
	if err != nil {
		return err
	}

	// always disable cgo if not native platform or not go compiler
	if !isNativeBuildPlatform {
		enableCgo = false
	}

	// build tags
	buildTags = slices.Clone(buildTags)
	buildTags = append(buildTags, NewBuildTags(buildType, enableCgo)...)

	// ldflags
	ldFlags = slices.Clone(ldFlags)

	// relative output path
	outBinPathRel, err := filepath.Rel(workingPath, outBinPath)
	if err != nil {
		return err
	}
	outBinDirRel, outBinFilename := filepath.Dir(outBinPathRel), filepath.Base(outBinPathRel)

	// args
	cmd := "go"
	args := append([]string{
		"build",
		"-trimpath",
		"-o",
		outBinPathRel,
	}, GetDefaultArgs()...)
	args = append(args, "-tags="+strings.Join(buildTags, ","))

	// if release or not native platform drop debugging symbols
	if isRelease || !isNativeBuildPlatform {
		ldFlags = append(ldFlags, "-w", "-s")
	}

	// ldflags
	if len(ldFlags) != 0 {
		args = append(args, "-ldflags", strings.Join(ldFlags, " "))
	}

	// module path
	args = append(args, ".")

	// go build
	ecmd := NewGoCompilerCmd(cmd, args...)
	ecmd.Dir = workingPath
	if enableCgo {
		ecmd.Env = append(ecmd.Env, "CGO_ENABLED=1")
	} else {
		ecmd.Env = append(ecmd.Env, "CGO_ENABLED=0")
	}
	ecmd.Env = append(ecmd.Env, platformEnv...)

	err = ExecGoCompiler(le, ecmd)
	if err != nil {
		return err
	}

	// post-processing in release mode
	if isWebBuildPlatform && isRelease {
		// track file size savings
		preOptStat, err := os.Stat(outBinPath)
		if err != nil {
			return err
		}
		preOptSize := preOptStat.Size()

		// wasm-opt
		// wasm-opt -Oz -o ./out.wasm.opt ./out.wasm
		optFilename := outBinFilename + ".wasm-opt"
		optPathRel := filepath.Join(outBinDirRel, optFilename)
		optPath := filepath.Join(workingPath, optPathRel)

		// -Os: optimized .wasm binary from 34580687 -> 32068818 bytes delta -2511869
		// -Oz: optimized .wasm binary from 34580687 -> 29498128 bytes delta -5082559
		ecmd := uexec.NewCmd("wasm-opt", "--enable-bulk-memory", "-Oz", "-o", optPathRel, outBinPathRel)
		ecmd.Env = os.Environ()
		ecmd.Dir = workingPath
		if err := ExecCmd(le, ecmd); err != nil {
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

		le.Infof("optimized .wasm binary from %d -> %d bytes delta %d", preOptSize, postOptSize, postOptSize-preOptSize)
	}

	return nil
}
