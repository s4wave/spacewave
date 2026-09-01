package gocompiler

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_platform_go "github.com/s4wave/spacewave/bldr/platform/go"
	opt_wasm "github.com/s4wave/spacewave/bldr/util/opt/wasm"
	"github.com/sirupsen/logrus"
)

var tinyGoBuildMu sync.Mutex

// shouldDropWasmDebugSymbols reports whether the non-TinyGo Go build must pass
// -w -s. Diagnostic mode keeps the linker name section for CPU-profile
// symbolization, but only for WASM (non-native) output: native release builds
// always strip regardless of the diagnostic flag.
func shouldDropWasmDebugSymbols(isRelease, isNativeBuildPlatform, isWasmOutput, diagnostic bool) bool {
	if isNativeBuildPlatform {
		return isRelease
	}
	if isWasmOutput && diagnostic {
		return false
	}
	return isRelease || !isNativeBuildPlatform
}

// shouldRunWasmOpt reports whether the release Go wasm build should run the
// Binaryen wasm-opt pass. Diagnostic mode bypasses it so the linker name
// section (and any DWARF) survives for CPU-profile symbolization.
func shouldRunWasmOpt(diagnostic bool) (bool, error) {
	if diagnostic {
		return false, nil
	}
	return GoWasmOptimizeEnabled()
}

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
	isNativeBuildPlatform := buildPlatform.GetBasePlatformID() == bldr_platform.PlatformID_DESKTOP
	isWasmOutput := strings.HasSuffix(outBinPath, ".wasm")
	goWasmDiagnostic, err := GoWasmDiagnosticEnabled()
	if err != nil {
		return err
	}

	platformEnv, err := bldr_platform_go.PlatformToGoEnv(buildPlatform)
	if err != nil {
		return err
	}

	// always disable cgo if not native platform or not go compiler or webassembly
	if !isNativeBuildPlatform || isWasmOutput {
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

		// if release or not native platform drop debugging symbols.
		// Diagnostic mode keeps the linker name section for CPU-profile
		// symbolization; release/default output is unchanged.
		if shouldDropWasmDebugSymbols(isRelease, isNativeBuildPlatform, isWasmOutput, goWasmDiagnostic) {
			ldFlags = append(ldFlags, "-w", "-s")
		}

		args = append(args, "-tags="+strings.Join(buildTags, ","))
	} else {
		cmd = "tinygo"
		args, err = newTinyGoBuildArgs(buildPlatform, buildType, outBinPathRel, buildTags)
		if err != nil {
			return err
		}
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

	timeStart := time.Now()
	if useTinygo {
		// TinyGo's LLVM pipeline can exhaust GitHub Linux runner memory when
		// multiple browser plugin compiles run inside one Bldr process.
		err = func() error {
			tinyGoBuildMu.Lock()
			defer tinyGoBuildMu.Unlock()
			return ExecGoCompiler(le, ecmd)
		}()
	} else {
		err = ExecGoCompiler(le, ecmd)
	}
	if err != nil {
		return err
	}
	le.
		WithField("compiler", cmd).
		WithField("dur", time.Since(timeStart).String()).
		Info("compiled plugin binary")

	// codesign the produced binary on darwin when BLDR_MACOS_SIGN_IDENTITY is set
	if !useTinygo && !isWasmOutput && slices.Contains(platformEnv, "GOOS=darwin") {
		if err := CodesignMacOS(ctx, le, outBinPath); err != nil {
			return err
		}
	}

	// az sign the produced binary on windows when BLDR_WINDOWS_SIGN_PROFILE is set
	if !useTinygo && !isWasmOutput && slices.Contains(platformEnv, "GOOS=windows") {
		if err := SignWindows(ctx, le, outBinPath); err != nil {
			return err
		}
	}

	// post-processing in release mode
	if isWasmOutput && isRelease {
		if useTinygo {
			tinygoDebugInfo, err := TinyGoDebugInfoEnabled()
			if err != nil {
				return err
			}
			if tinygoDebugInfo {
				le.Info("kept TinyGo wasm debug sections")
			} else {
				if err := opt_wasm.StripWasmDebugSections(ctx, le, workingPath, outBinPath); err != nil {
					return err
				}
			}
		} else {
			optimize, err := shouldRunWasmOpt(goWasmDiagnostic)
			if err != nil {
				return err
			}
			if !optimize {
				le.Info("skipped wasm-opt for Go wasm output")
				return nil
			}
			if err := opt_wasm.OptimizeWasmBinary(ctx, le, workingPath, outBinPath); err != nil {
				return err
			}
		}
	}

	return nil
}

const tinyGoInternalNoDWARFArg = "-internal-nodwarf"

func newTinyGoBuildArgs(
	buildPlatform bldr_platform.Platform,
	buildType bldr_manifest.BuildType,
	outBinPathRel string,
	buildTags []string,
) ([]string, error) {
	tinygoPlat, err := bldr_platform_go.PlatformToTinyGoTarget(buildPlatform)
	if err != nil {
		return nil, err
	}
	tinygoArgs, err := GetDefaultTinygoArgs()
	if err != nil {
		return nil, err
	}
	tinygoDebugInfo, err := TinyGoDebugInfoEnabled()
	if err != nil {
		return nil, err
	}
	buildTags = slices.Clone(buildTags)
	if shouldUseBldrTinyGoJSImportBuildTag(buildPlatform, tinygoPlat) {
		buildTags = append(buildTags, BldrTinyGoJSImportBuildTag)
	}

	args := append([]string{
		"build",
		"-o",
		outBinPathRel,
		"-target", tinygoPlat,
	}, tinygoArgs...)

	// Browser staging investigations sometimes need TinyGo DWARF symbols;
	// otherwise release and non-native builds drop debug info to keep wasm small.
	if !tinygoDebugInfo && (buildType.IsRelease() || buildPlatform.GetBasePlatformID() != bldr_platform.PlatformID_DESKTOP) {
		args = append(args, "-no-debug")
	}
	if !tinygoDebugInfo && shouldSkipTinyGoInternalDWARF(buildPlatform, buildType) {
		args = append(args, tinyGoInternalNoDWARFArg)
	}

	args = append(args, "-tags="+strings.Join(buildTags, " "))
	return args, nil
}

func shouldUseBldrTinyGoJSImportBuildTag(buildPlatform bldr_platform.Platform, tinygoPlat string) bool {
	return buildPlatform.GetBasePlatformID() == bldr_platform.PlatformID_WEB && tinygoPlat == "wasm"
}

func shouldSkipTinyGoInternalDWARF(buildPlatform bldr_platform.Platform, buildType bldr_manifest.BuildType) bool {
	if !buildType.IsRelease() {
		return false
	}
	np := bldr_platform.ToNativePlatform(buildPlatform)
	if np == nil {
		return false
	}
	return np.GetGOOS() == "js" && np.GetGOARCH() == "wasm"
}
