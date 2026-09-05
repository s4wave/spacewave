//go:build !js

package bldr_dist_compiler

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
)

// TestNativeAssetPackageCache measures the real Go package archive generated
// from the native entrypoint, using the configured shared build cache.
func TestNativeAssetPackageCache(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a native entrypoint")
	}
	if err := os.MkdirAll(".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(".tmp", "native-cache-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	meta := bldr_dist.NewDistMeta("cache-fixture", "desktop/darwin/arm64", nil, nil, "dist")
	if err := os.WriteFile(filepath.Join(dir, "config-set.bin"), []byte("config"), 0o644); err != nil {
		t.Fatal(err)
	}
	asset := bytes.Repeat([]byte("large-asset-data!"), 512*1024)
	measure := func(embed bool) (string, int64) {
		t.Helper()
		files := []string{"config-set.bin"}
		if embed {
			files = append(files, "assets.kvfile")
		}
		if err := os.WriteFile(filepath.Join(dir, "assets.kvfile"), asset, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(FormatDistEntrypoint(meta, files, nil, true)), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(t.Context(), "go", "list", "-mod=mod", "-export", "-f", "{{.Export}}", ".")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("compile native entrypoint: %v\n%s", err, out)
		}
		archive := strings.TrimSpace(string(out))
		info, err := os.Stat(archive)
		if err != nil {
			t.Fatal(err)
		}
		return archive, info.Size()
	}
	_, before := measure(true)
	first, after := measure(false)
	asset[0]++
	second, changed := measure(false)
	if before < int64(len(asset)) || after >= 1024*1024 || changed != after || first != second {
		t.Fatalf("archive bytes: embedded=%d external=%d changed=%d; external cache reused=%v", before, after, changed, first == second)
	}
	t.Logf("asset=%d bytes; embedded archive=%d; external archive=%d; asset-only edit reuses archive", len(asset), before, after)
}
