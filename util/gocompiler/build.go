package gocompiler

import (
	"path/filepath"
	"slices"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	bldr_platform_go "github.com/aperturerobotics/bldr/platform/go"
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
	outBinPath, err = filepath.Rel(workingPath, outBinPath)
	if err != nil {
		return err
	}

	// args
	cmd := "go"
	args := append([]string{
		"build",
		"-trimpath",
		"-o",
		outBinPath,
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

	return ExecGoCompiler(le, ecmd)
}
