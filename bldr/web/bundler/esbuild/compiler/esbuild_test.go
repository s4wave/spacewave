//go:build !js

package bldr_web_bundler_esbuild_compiler

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	bldr_web_bundler_esbuild "github.com/s4wave/spacewave/bldr/web/bundler/esbuild"
	"github.com/sirupsen/logrus"
)

func TestBuildEsbuildBundlePreservesConfiguredPluginContract(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "assets")
	writeEsbuildControlFile(t, filepath.Join(root, "entry.ts"), `
import { used } from './values'
globalThis.__bldrEsbuildControl = used
`)
	writeEsbuildControlFile(t, filepath.Join(root, "values.ts"), `
export const used = 'retained-sentinel'
export const unused = 'unused-sentinel'
`)

	webPkgs, outputs, inputs, err := BuildEsbuildBundle(
		logrus.NewEntry(logrus.New()),
		root,
		"control",
		[]*bldr_web_bundler_esbuild.EsbuildBundleEntrypoint{{
			InputPath:    "entry.ts",
			OutputPath:   "control",
			EntrypointId: "control-entrypoint",
		}},
		&esbuild_api.BuildOptions{OutExtension: map[string]string{".js": ".mjs"}},
		nil,
		assets,
		"/p/control/esb/",
		false,
		true,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(webPkgs) != 0 {
		t.Fatalf("web package refs = %v, want none", webPkgs)
	}
	for _, input := range []string{"entry.ts", "values.ts"} {
		if !slices.Contains(inputs, input) {
			t.Fatalf("inputs = %v, missing %q", inputs, input)
		}
	}

	outputIndex := slices.IndexFunc(outputs, func(output *bldr_web_bundler_esbuild.EsbuildOutputMeta) bool {
		return output.GetEntrypointId() == "control-entrypoint"
	})
	if outputIndex == -1 {
		t.Fatalf("outputs = %v, missing configured entrypoint metadata", outputs)
	}
	output := outputs[outputIndex]
	if output.GetEntrypointPath() != "entry.ts" {
		t.Fatalf("entrypoint path = %q, want entry.ts", output.GetEntrypointPath())
	}
	if filepath.IsAbs(output.GetPath()) || strings.HasPrefix(filepath.ToSlash(output.GetPath()), "../") {
		t.Fatalf("output path = %q, want asset-root-relative path", output.GetPath())
	}
	outputPath := filepath.Join(assets, output.GetPath())
	code, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(code, []byte("retained-sentinel")) {
		t.Fatalf("output missing retained export: %s", code)
	}
	if bytes.Contains(code, []byte("unused-sentinel")) {
		t.Fatalf("output retained unused export: %s", code)
	}
	if output.GetLength() != uint32(len(code)) {
		t.Fatalf("metadata length = %d, output length = %d", output.GetLength(), len(code))
	}

	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	if _, err := gzipWriter.Write(code); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf(
		"configured esbuild control: output=%s raw=%d gzip=%d inputs=%d outputs=%d",
		output.GetPath(),
		len(code),
		compressed.Len(),
		len(inputs),
		len(outputs),
	)
}

func writeEsbuildControlFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
