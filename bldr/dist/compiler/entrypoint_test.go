package bldr_dist_compiler

import (
	"errors"
	"strings"
	"testing"

	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
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

	_, err = resolveDistGoCompiler(platform, plugin_compiler_go.GoCompiler_GO_COMPILER_GOSCRIPT)
	if err == nil {
		t.Fatal("expected GoScript dist compiler error")
	}
	if !errors.Is(err, errGoScriptDistUnsupported) {
		t.Fatalf("error = %v, want %v", err, errGoScriptDistUnsupported)
	}
}
