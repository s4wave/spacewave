package gocompiler

import (
	"slices"
	"strings"
	"testing"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

func TestNewBuildTagsDoNotDependOnReleaseEnv(t *testing.T) {
	for _, env := range []string{"", "prod", "staging"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("SPACEWAVE_RELEASE_ENV", env)
			tags := NewBuildTags(bldr_manifest.BuildType_RELEASE, false)
			if !slices.Equal(tags, []string{"build_type_release", "purego"}) {
				t.Fatalf("build tags = %v, want release defaults only", tags)
			}
		})
	}
}

func TestDefaultTinyGoArgsTrapOnPanic(t *testing.T) {
	clearTinyGoOptionEnv(t)

	args, err := GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(args, "-panic=trap") {
		t.Fatalf("tinygo args = %v, want -panic=trap", args)
	}
}

func TestFastTinyGoProfileDropsBrokenOptZero(t *testing.T) {
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoProfileEnv, TinyGoProfileFast)

	args, err := GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(args, "-opt=1") {
		t.Fatalf("tinygo args = %v, want -opt=1", args)
	}
	if slices.Contains(args, "-opt=0") {
		t.Fatalf("fast TinyGo profile should not use broken -opt=0 by default: %v", args)
	}
	if !slices.Contains(args, "-interp-timeout=10m") {
		t.Fatalf("tinygo args = %v, want -interp-timeout=10m", args)
	}
	if !slices.Contains(args, "-gc=leaking") {
		t.Fatalf("tinygo args = %v, want -gc=leaking", args)
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-scheduler=") {
			t.Fatalf("fast TinyGo profile should not set scheduler by default: %v", args)
		}
	}
}

func TestOptimizedTinyGoProfileUsesOptTwo(t *testing.T) {
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoProfileEnv, TinyGoProfileOptimized)

	args, err := GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(args, "-opt=2") {
		t.Fatalf("tinygo args = %v, want -opt=2", args)
	}
}

func TestTinyGoSchedulerIsExplicit(t *testing.T) {
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoProfileEnv, TinyGoProfileFast)
	t.Setenv(TinyGoSchedulerEnv, "none")

	args, err := GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(args, "-scheduler=none") {
		t.Fatalf("tinygo args = %v, want -scheduler=none", args)
	}
}

func TestDefaultTinyGoArgsPrintsPanicWhenConfigured(t *testing.T) {
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoPanicStrategyEnv, "print")

	args, err := GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(args, "-panic=print") {
		t.Fatalf("tinygo args = %v, want -panic=print", args)
	}
}

func TestDefaultTinyGoArgsRejectsUnknownPanicStrategy(t *testing.T) {
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoPanicStrategyEnv, "bogus")

	_, err := GetDefaultTinygoArgs()
	if err == nil {
		t.Fatal("expected invalid TinyGo panic strategy to fail")
	}
	if !strings.Contains(err.Error(), TinyGoPanicStrategyEnv) {
		t.Fatalf("error = %q, want %s", err.Error(), TinyGoPanicStrategyEnv)
	}
}

func TestTinyGoArgsUseExplicitIdentityInputs(t *testing.T) {
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoProfileEnv, TinyGoProfileFast)
	t.Setenv(TinyGoOptEnv, "1")
	t.Setenv(TinyGoGCEnv, "leaking")
	t.Setenv(TinyGoLLVMFeaturesEnv, "+atomics,+bulk-memory")

	args, err := GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"-opt=1",
		"-gc=leaking",
		"-llvm-features=+atomics,+bulk-memory",
		"-interp-timeout=10m",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("tinygo args = %v, want %s", args, want)
		}
	}
}

func clearTinyGoOptionEnv(t *testing.T) {
	t.Helper()
	for _, key := range TinyGoStartupCacheEnvKeys() {
		t.Setenv(key, "")
	}
}
