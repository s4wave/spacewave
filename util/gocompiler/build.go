package gocompiler

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	bldr_platform_go "github.com/aperturerobotics/bldr/platform/go"
	opt_wasm "github.com/aperturerobotics/bldr/util/opt/wasm"
	"github.com/sirupsen/logrus"
)

// ExecBuildEntrypoint executes building an entrypoint main package.
func ExecBuildEntrypoint(
	ctx context.Context,
	le *logrus.Entry,
	buildPlatform bldr_platform.Platform,
	buildType bldr_manifest.BuildType,
	workingPath,
	outBinPath string,
	enableCgo bool,
	useTinygo bool,
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

	// args
	var cmd string
	var args []string
	if !useTinygo {
		cmd = "go"
		args = append([]string{
			"build",
			"-trimpath",
			"-o",
			outBinPathRel,
		}, GetDefaultArgs()...)

		// if release or not native platform drop debugging symbols
		if isRelease || !isNativeBuildPlatform {
			ldFlags = append(ldFlags, "-w", "-s")
		}

		args = append(args, "-tags="+strings.Join(buildTags, ","))
	} else {
		cmd = "tinygo"
		tinygoPlat, err := bldr_platform_go.PlatformToTinyGoTarget(buildPlatform)
		if err != nil {
			return err
		}
		args = append([]string{
			"build",
			"-o",
			outBinPathRel,
			"-target", tinygoPlat,
		}, GetDefaultTinygoArgs()...)

		// if release or not native platform drop debugging symbols
		if isRelease || !isNativeBuildPlatform {
			args = append(args, "-no-debug")
		}

		args = append(args, "-tags="+strings.Join(buildTags, " "))
	}

	// ldflags
	if len(ldFlags) != 0 {
		args = append(args, "-ldflags", strings.Join(ldFlags, " "))
	}

	// module path
	args = append(args, ".")

	// go build
	ecmd := NewGoCompilerCmd(ctx, cmd, args...)
	ecmd.Dir = workingPath
	if !useTinygo {
		if enableCgo {
			ecmd.Env = append(ecmd.Env, "CGO_ENABLED=1")
		} else {
			ecmd.Env = append(ecmd.Env, "CGO_ENABLED=0")
		}
		ecmd.Env = append(ecmd.Env, platformEnv...)
	}

	err = ExecGoCompiler(le, ecmd)
	if err != nil {
		return err
	}

	// post-processing in release mode
	if isWebBuildPlatform && isRelease {
		if err := opt_wasm.OptimizeWasmBinary(ctx, le, workingPath, outBinPath); err != nil {
			return err
		}
	}

	return nil
}
