package bldr_dist_compiler

import (
	"slices"
	"strings"
	"testing"

	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
)

func TestFormatDistEntrypointNativeCLI(t *testing.T) {
	meta := bldr_dist.NewDistMeta("spacewave", "desktop/darwin/arm64", nil, nil, "dist")
	src := FormatDistEntrypoint(
		meta,
		[]string{"assets.kvfile", "config-set.bin"},
		map[string]string{
			"github.com/s4wave/spacewave/cmd/spacewave/cli": "spacewave_cli",
		},
		true,
	)

	if !strings.Contains(src, `cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"`) {
		t.Fatalf("expected native CLI import, got:\n%s", src)
	}
	if !strings.Contains(src, `spacewave_cli "github.com/s4wave/spacewave/cmd/spacewave/cli"`) {
		t.Fatalf("expected CLI package import, got:\n%s", src)
	}
	if !strings.Contains(src, `var cliCommands = []cli_entrypoint.BuildCommandsFunc{spacewave_cli.NewCliCommands}`) {
		t.Fatalf("expected cliCommands declaration, got:\n%s", src)
	}
	if !strings.Contains(src, `dist_entrypoint.Main(DistMeta, LogLevel, AssetsFS, cliCommands)`) {
		t.Fatalf("expected native main call to pass cliCommands, got:\n%s", src)
	}
}

func TestFormatDistEntrypointWeb(t *testing.T) {
	meta := bldr_dist.NewDistMeta("spacewave", "web/js/wasm", nil, nil, "dist")
	src := FormatDistEntrypoint(meta, []string{"assets.url"}, nil, false)

	if strings.Contains(src, "cli_entrypoint") {
		t.Fatalf("did not expect CLI imports in web entrypoint, got:\n%s", src)
	}
	if strings.Contains(src, "cliCommands") {
		t.Fatalf("did not expect CLI declarations in web entrypoint, got:\n%s", src)
	}
	if !strings.Contains(src, `dist_entrypoint.Main(DistMeta, LogLevel, AssetsFS)`) {
		t.Fatalf("expected web main call without cliCommands, got:\n%s", src)
	}
}

func TestDistEntrypointLDFlags(t *testing.T) {
	for _, tc := range []struct {
		platformID string
		role       string
		want       []string
	}{
		{"desktop/windows/amd64", bldr_dist.EntrypointRoleDesktop, []string{"-H=windowsgui"}},
		{"desktop/windows/amd64", bldr_dist.EntrypointRoleCLI, nil},
		{"desktop/darwin/arm64", bldr_dist.EntrypointRoleDesktop, nil},
	} {
		platform, err := bldr_platform.ParsePlatform(tc.platformID)
		if err != nil {
			t.Fatal(err)
		}
		if got := distEntrypointLDFlags(platform, tc.role); !slices.Equal(got, tc.want) {
			t.Fatalf("dist entrypoint ldflags for %s/%s = %v, want %v", tc.platformID, tc.role, got, tc.want)
		}
	}
}

func TestResolveDistGoCompiler(t *testing.T) {
	platform, err := bldr_platform.ParsePlatform("web/js/wasm")
	if err != nil {
		t.Fatal(err)
	}

	goCompiler, err := resolveDistGoCompiler(platform, plugin_compiler_go.GoCompiler_GO_COMPILER_TINYGO)
	if err != nil {
		t.Fatal(err)
	}
	if goCompiler != gocompiler.GoCompilerTinyGo {
		t.Fatalf("goCompiler = %s, want %s", goCompiler, gocompiler.GoCompilerTinyGo)
	}

	goCompiler, err = resolveDistGoCompiler(platform, plugin_compiler_go.GoCompiler_GO_COMPILER_GOSCRIPT)
	if err != nil {
		t.Fatal(err)
	}
	if goCompiler != gocompiler.GoCompilerGoScript {
		t.Fatalf("goCompiler = %s, want %s", goCompiler, gocompiler.GoCompilerGoScript)
	}
}

// TestNewDistGoScriptBuildFlags verifies opt-in startup trace propagation for GoScript.
func TestNewDistGoScriptBuildFlags(t *testing.T) {
	t.Setenv(gocompiler.RuntimeStartupTraceEnv, "")
	flags := strings.Join(newDistGoScriptBuildFlags(bldr_manifest.BuildType_RELEASE, false), " ")
	if !strings.Contains(flags, gocompiler.GoScriptBuildTag) {
		t.Fatalf("flags = %q, want %s tag", flags, gocompiler.GoScriptBuildTag)
	}
	if strings.Contains(flags, gocompiler.RuntimeStartupTraceBuildTag) {
		t.Fatalf("flags = %q, unexpected %s tag", flags, gocompiler.RuntimeStartupTraceBuildTag)
	}

	t.Setenv(gocompiler.RuntimeStartupTraceEnv, "1")
	flags = strings.Join(newDistGoScriptBuildFlags(bldr_manifest.BuildType_RELEASE, false), " ")
	if !strings.Contains(flags, gocompiler.RuntimeStartupTraceBuildTag) {
		t.Fatalf("flags = %q, want %s tag", flags, gocompiler.RuntimeStartupTraceBuildTag)
	}
}

func TestNewDistGoScriptEnvUsesWebPlatform(t *testing.T) {
	platform, err := bldr_platform.ParsePlatform("web/js/wasm")
	if err != nil {
		t.Fatal(err)
	}
	env, err := newDistGoScriptEnv(platform)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"GOOS=js", "GOARCH=wasm"} {
		if !slices.Contains(env, want) {
			t.Fatalf("env = %v, want %s", env, want)
		}
	}
}
