//go:build goscript_profile

package goscriptbench

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const profileHarnessTest = `package profile

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/s4wave/goscript/compiler"
)

func TestSpacewaveCoreCompile(t *testing.T) {
	spacewaveDir := os.Getenv("SPACEWAVE_REPO")
	if spacewaveDir == "" {
		t.Fatal("SPACEWAVE_REPO is required")
	}
	out := os.Getenv("GOSCRIPT_OUTPUT")
	if out == "" {
		out = filepath.Join(spacewaveDir, ".tmp", "goscript-profile-bench", "out")
	}
	if err := os.RemoveAll(out); err != nil {
		t.Fatal(err)
	}

	comp, err := compiler.NewCompiler(&compiler.Config{
		Dir:            spacewaveDir,
		OutputPath:     out,
		BuildFlags:     []string{"-tags=goscript,skip_e2e,purego"},
		OverrideDirs:   []string{filepath.Join(spacewaveDir, "gs")},
		AllDependencies: true,
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = comp.CompilePackages(context.Background(),
		"./core/resource/root/controller",
		"./core/resource/listener",
		"./core/session/controller",
		"./core/provider/local",
		"./core/provider/spacewave",
		"./core/space/sobject",
		"./core/sobject/world/engine",
		"./core/space/world/optypes",
		"./core/plugin/space",
		"./core/space/http/download",
		"./core/space/http/export",
		"./db/blocktype/controller-factory",
		"github.com/s4wave/spacewave/db/object/peer",
	)
	if err != nil {
		t.Fatal(err)
	}
}
`

func TestSpacewaveCoreGoScriptProfileHarness(t *testing.T) {
	root := mustRepoRoot(t)
	profileDir := filepath.Join(root, ".tmp", "goscript-profile-bench", "test")
	if err := os.RemoveAll(profileDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}

	runProfileHarness(t, root, profileDir, filepath.Join(profileDir, "out"), false)
}

func BenchmarkSpacewaveCoreGoScriptCompile(b *testing.B) {
	root := mustRepoRoot(b)
	profileRoot := filepath.Join(root, ".tmp", "goscript-profile-bench")
	if err := os.MkdirAll(profileRoot, 0o755); err != nil {
		b.Fatal(err)
	}

	var lastOut string
	for i := 0; i < b.N; i++ {
		runDir := filepath.Join(profileRoot, fmt.Sprintf("bench-%d", i))
		if err := os.RemoveAll(runDir); err != nil {
			b.Fatal(err)
		}
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			b.Fatal(err)
		}
		out := filepath.Join(runDir, "out")
		runProfileHarness(b, root, runDir, out, true)
		lastOut = out
	}

	b.StopTimer()
	if lastOut != "" {
		files, bytes, err := outputStats(lastOut)
		if err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(float64(files), "files/op")
		b.ReportMetric(float64(bytes)/(1024*1024), "MiB/op")
	}
}

func runProfileHarness(tb testing.TB, root string, profileDir string, outputDir string, profile bool) {
	tb.Helper()

	harnessDir := tb.TempDir()
	goscriptRepo := os.Getenv("GOSCRIPT_REPO")
	if goscriptRepo == "" {
		goscriptRepo = filepath.Clean(filepath.Join(root, "..", "goscript"))
	}
	goscriptRepoAbs, err := filepath.Abs(goscriptRepo)
	if err != nil {
		tb.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(goscriptRepoAbs, "go.mod")); err != nil {
		tb.Fatalf("GOSCRIPT_REPO %q is not a Go module: %v", goscriptRepoAbs, err)
	}

	goMod := "module spacewave-goscript-profile\n\ngo 1.25.3\n\n" +
		"require github.com/s4wave/goscript v0.0.0\n\n" +
		"replace github.com/s4wave/goscript => " + filepath.ToSlash(goscriptRepoAbs) + "\n"
	if err := os.MkdirAll(harnessDir, 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(harnessDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(harnessDir, "profile_test.go"), []byte(profileHarnessTest), 0o644); err != nil {
		tb.Fatal(err)
	}

	args := []string{"test", "-run", "^TestSpacewaveCoreCompile$", "-count=1", "-timeout=15m"}
	if profile {
		args = append(args,
			"-cpuprofile", filepath.Join(profileDir, "cpu.pprof"),
			"-memprofile", filepath.Join(profileDir, "mem.pprof"),
		)
	}
	args = append(args, ".")

	cmd := exec.Command("go", args...)
	cmd.Dir = harnessDir
	cmd.Env = append(os.Environ(),
		"SPACEWAVE_REPO="+root,
		"GOSCRIPT_OUTPUT="+outputDir,
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		tb.Fatalf("goscript profile harness failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if err := os.WriteFile(filepath.Join(profileDir, "go-test.log"), stdout.Bytes(), 0o644); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "go-test.err"), stderr.Bytes(), 0o644); err != nil {
		tb.Fatal(err)
	}
}

func mustRepoRoot(tb testing.TB) string {
	tb.Helper()
	dir, err := os.Getwd()
	if err != nil {
		tb.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			tb.Fatal("could not find repository root")
		}
		dir = next
	}
}

func outputStats(root string) (int, int64, error) {
	if strings.TrimSpace(root) == "" {
		return 0, 0, fmt.Errorf("empty output root")
	}

	var files int
	var bytes int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files++
		bytes += info.Size()
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return files, bytes, nil
}
