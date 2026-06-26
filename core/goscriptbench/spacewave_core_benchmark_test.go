//go:build goscript_profile

package goscriptbench

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/pkg/errors"
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

type runMode int

const (
	runUntraced runMode = iota
	runProfiled
	runTraced
)

func TestSpacewaveCoreGoScriptProfileHarness(t *testing.T) {
	root := mustRepoRoot(t)
	profileDir := filepath.Join(root, ".tmp", "goscript-profile-bench", "test")
	if err := os.RemoveAll(profileDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}

	runProfileHarness(t, root, profileDir, filepath.Join(profileDir, "out"), runUntraced)
}

func TestGoTestArgsRunMode(t *testing.T) {
	profileDir := filepath.Join("profiles")
	cases := []struct {
		name string
		mode runMode
		want []string
	}{
		{
			name: "untraced",
			mode: runUntraced,
			want: []string{"test", "-run", "^TestSpacewaveCoreCompile$", "-count=1", "-timeout=15m", "."},
		},
		{
			name: "profiled",
			mode: runProfiled,
			want: []string{
				"test", "-run", "^TestSpacewaveCoreCompile$", "-count=1", "-timeout=15m",
				"-cpuprofile", filepath.Join(profileDir, "cpu.pprof"),
				"-memprofile", filepath.Join(profileDir, "mem.pprof"),
				".",
			},
		},
		{
			name: "traced",
			mode: runTraced,
			want: []string{
				"test", "-run", "^TestSpacewaveCoreCompile$", "-count=1", "-timeout=15m",
				"-cpuprofile", filepath.Join(profileDir, "cpu.pprof"),
				"-memprofile", filepath.Join(profileDir, "mem.pprof"),
				"-trace", filepath.Join(profileDir, "trace.out"),
				".",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := goTestArgs(profileDir, tc.mode)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("args mismatch\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

func TestOutputStatsTreeHashDeterministic(t *testing.T) {
	first := t.TempDir()
	writeOutputFile(t, first, "b/out.ts", "beta")
	writeOutputFile(t, first, "a/out.ts", "alpha")

	second := t.TempDir()
	writeOutputFile(t, second, "a/out.ts", "alpha")
	writeOutputFile(t, second, "b/out.ts", "beta")

	files, bytes, hash, err := outputStats(first)
	if err != nil {
		t.Fatal(err)
	}
	if files != 2 {
		t.Fatalf("files = %d, want 2", files)
	}
	if bytes != int64(len("alpha")+len("beta")) {
		t.Fatalf("bytes = %d, want %d", bytes, len("alpha")+len("beta"))
	}
	_, _, sameHash, err := outputStats(second)
	if err != nil {
		t.Fatal(err)
	}
	if hash != sameHash {
		t.Fatalf("hash differs for identical trees: %s != %s", hash, sameHash)
	}

	h := sha256.New()
	writeHashPart(h, []byte("a/out.ts"))
	writeHashPart(h, []byte("alpha"))
	writeHashPart(h, []byte("b/out.ts"))
	writeHashPart(h, []byte("beta"))
	want := hex.EncodeToString(h.Sum(nil))
	if hash != want {
		t.Fatalf("hash = %s, want %s", hash, want)
	}

	renamed := t.TempDir()
	writeOutputFile(t, renamed, "a/out.ts", "alpha")
	writeOutputFile(t, renamed, "c/out.ts", "beta")
	_, _, renamedHash, err := outputStats(renamed)
	if err != nil {
		t.Fatal(err)
	}
	if renamedHash == hash {
		t.Fatal("hash did not include relative paths")
	}
}

func writeOutputFile(tb testing.TB, root string, rel string, content string) {
	tb.Helper()

	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		tb.Fatal(err)
	}
}

func BenchmarkSpacewaveCoreGoScriptCompile(b *testing.B) {
	root := mustRepoRoot(b)
	profileRoot := filepath.Join(root, ".tmp", "goscript-profile-bench")
	if err := os.MkdirAll(profileRoot, 0o755); err != nil {
		b.Fatal(err)
	}

	var lastOut string
	for i := 0; i < b.N; i++ {
		runDir := filepath.Join(profileRoot, "bench-"+strconv.Itoa(i))
		if err := os.RemoveAll(runDir); err != nil {
			b.Fatal(err)
		}
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			b.Fatal(err)
		}
		out := filepath.Join(runDir, "out")
		runProfileHarness(b, root, runDir, out, runUntraced)
		lastOut = out
	}

	b.StopTimer()
	if lastOut != "" {
		files, bytes, treeHash, err := outputStats(lastOut)
		if err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(float64(files), "files/op")
		b.ReportMetric(float64(bytes)/(1024*1024), "MiB/op")
		b.Logf("tree_hash=%s", treeHash)
	}
}

func runProfileHarness(tb testing.TB, root string, profileDir string, outputDir string, mode runMode) {
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

	args, err := goTestArgs(profileDir, mode)
	if err != nil {
		tb.Fatal(err)
	}

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

func goTestArgs(profileDir string, mode runMode) ([]string, error) {
	args := []string{"test", "-run", "^TestSpacewaveCoreCompile$", "-count=1", "-timeout=15m"}
	switch mode {
	case runUntraced:
	case runProfiled:
		args = append(args,
			"-cpuprofile", filepath.Join(profileDir, "cpu.pprof"),
			"-memprofile", filepath.Join(profileDir, "mem.pprof"),
		)
	case runTraced:
		args = append(args,
			"-cpuprofile", filepath.Join(profileDir, "cpu.pprof"),
			"-memprofile", filepath.Join(profileDir, "mem.pprof"),
			"-trace", filepath.Join(profileDir, "trace.out"),
		)
	default:
		return nil, errors.Errorf("unknown run mode: %d", mode)
	}
	return append(args, "."), nil
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

func outputStats(root string) (int, int64, string, error) {
	if strings.TrimSpace(root) == "" {
		return 0, 0, "", errors.New("empty output root")
	}

	var files int
	var bytes int64
	hash := sha256.New()
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
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files++
		bytes += info.Size()
		writeHashPart(hash, []byte(filepath.ToSlash(rel)))
		writeHashPart(hash, content)
		return nil
	})
	if err != nil {
		return 0, 0, "", err
	}
	return files, bytes, hex.EncodeToString(hash.Sum(nil)), nil
}

func writeHashPart(h hash.Hash, data []byte) {
	_, _ = h.Write(strconv.AppendInt(nil, int64(len(data)), 10))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(data)
	_, _ = h.Write([]byte{0})
}
