//go:build !js

package bldr_dist_compiler

import (
	"slices"
	"strings"
	"testing"

	bldr_cli_compiler "github.com/s4wave/spacewave/bldr/cli/compiler"
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
		map[string]bldr_cli_compiler.CliImport{
			"example.com/app/cli": {Alias: "app_cli"},
		},
		bldr_manifest.BuildType_DEV,
		true,
		"",
		"example.com/app/compose",
	)

	for _, want := range []string{
		`project_compose "example.com/app/compose"`,
		`app_cli "example.com/app/cli"`,
		"var cliCommands = []cli_entrypoint.BuildCommandsFunc{app_cli.NewCliCommands}",
		"composition := project_compose.Compose()",
		"composition.Commands = append(composition.Commands, cliCommands...)",
		"dist_entrypoint.Main(DistMeta, LogLevel, AssetsFS, composition)",
		"var LogLevel = logrus.DebugLevel",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("native entrypoint omits %s:\n%s", want, src)
		}
	}
}

func TestFormatDistEntrypointWeb(t *testing.T) {
	meta := bldr_dist.NewDistMeta("spacewave", "web/js/wasm", nil, nil, "dist")
	src := FormatDistEntrypoint(
		meta,
		[]string{"assets.url"},
		nil,
		bldr_manifest.BuildType_RELEASE,
		false,
		"example.com/native",
		"example.com/app/compose",
	)

	for _, leak := range []string{"cli_entrypoint", "cliCommands", "native_runner"} {
		if strings.Contains(src, leak) {
			t.Fatalf("native %s leaked into browser entrypoint:\n%s", leak, src)
		}
	}
	for _, want := range []string{
		"composition := project_compose.Compose()",
		"dist_entrypoint.Main(DistMeta, LogLevel, AssetsFS, composition)",
		"var LogLevel = logrus.WarnLevel",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("web entrypoint omits %s:\n%s", want, src)
		}
	}
}

func TestFormatDistEntrypointNativeRunner(t *testing.T) {
	meta := bldr_dist.NewDistMeta("spacewave", "desktop/linux/amd64", nil, nil, "dist")
	src := FormatDistEntrypoint(
		meta,
		[]string{"assets.kvfile"},
		nil,
		bldr_manifest.BuildType_DEV,
		true,
		"example.com/app/tray",
		"",
	)
	for _, want := range []string{
		`native_runner "example.com/app/tray"`,
		"composition := &compose.Composition{}",
		"dist_entrypoint.MainWithRunner(DistMeta, LogLevel, AssetsFS, composition, native_runner.Run)",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("native runner entrypoint omits %s:\n%s", want, src)
		}
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
	flags := strings.Join(newDistGoScriptBuildFlags(bldr_manifest.BuildType_RELEASE), " ")
	if !strings.Contains(flags, gocompiler.GoScriptBuildTag) {
		t.Fatalf("flags = %q, want %s tag", flags, gocompiler.GoScriptBuildTag)
	}
	if strings.Contains(flags, gocompiler.RuntimeStartupTraceBuildTag) {
		t.Fatalf("flags = %q, unexpected %s tag", flags, gocompiler.RuntimeStartupTraceBuildTag)
	}

	t.Setenv(gocompiler.RuntimeStartupTraceEnv, "1")
	flags = strings.Join(newDistGoScriptBuildFlags(bldr_manifest.BuildType_RELEASE), " ")
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
