//go:build !js

package harness

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	resolveTestRoleEnv  = "SPACEWAVE_HARNESS_RESOLVE_TEST_ROLE"
	resolveTestRootEnv  = "SPACEWAVE_HARNESS_RESOLVE_TEST_ROOT"
	resolveTestKeyEnv   = "SPACEWAVE_HARNESS_RESOLVE_TEST_KEY"
	resolveTestFreshEnv = "SPACEWAVE_HARNESS_RESOLVE_TEST_FRESH"
)

type stubShape struct {
	root string
	key  string
}

func (s *stubShape) ContentKey(context.Context) (string, error) {
	return s.key, nil
}

func (s *stubShape) Lookup(context.Context, string) ([]Generation[string], error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "tokens", s.key))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	generations := make([]Generation[string], 0, len(entries))
	for _, entry := range entries {
		generations = append(generations, Generation[string]{Token: entry.Name(), Artifact: entry.Name()})
	}
	return generations, nil
}

func (s *stubShape) Build(context.Context, string) (Generation[string], error) {
	countPath := filepath.Join(s.root, "builds-"+s.key)
	data, err := os.ReadFile(countPath)
	if err != nil && !os.IsNotExist(err) {
		return Generation[string]{}, err
	}
	count := 0
	if len(data) != 0 {
		count, err = strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return Generation[string]{}, err
		}
	}
	count++
	if err := os.WriteFile(countPath, []byte(strconv.Itoa(count)+"\n"), 0o644); err != nil {
		return Generation[string]{}, err
	}
	token := s.key + "-" + strconv.Itoa(count)
	dir := filepath.Join(s.root, "tokens", s.key)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Generation[string]{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, token), []byte(token), 0o644); err != nil {
		return Generation[string]{}, err
	}
	return Generation[string]{Token: token, Artifact: token}, nil
}

type fixedShape struct {
	generations []Generation[string]
	builds      int
}

func (s *fixedShape) ContentKey(context.Context) (string, error) { return "key", nil }

func (s *fixedShape) Lookup(context.Context, string) ([]Generation[string], error) {
	return s.generations, nil
}

func (s *fixedShape) Build(context.Context, string) (Generation[string], error) {
	s.builds++
	return Generation[string]{Token: "built", Artifact: "built"}, nil
}

func TestResolveRejectsEmptyAndDuplicateTokens(t *testing.T) {
	for _, generations := range [][]Generation[string]{
		{{Token: "", Artifact: "empty"}},
		{{Token: "duplicate", Artifact: "a"}, {Token: "duplicate", Artifact: "b"}},
	} {
		shape := &fixedShape{generations: generations}
		if _, err := Resolve(context.Background(), nil, ResolveOptions{LockDir: t.TempDir(), LockName: "build"}, shape); err == nil {
			t.Fatal("Resolve accepted invalid lookup tokens")
		}
		if shape.builds != 0 {
			t.Fatalf("build count = %d, want 0", shape.builds)
		}
	}
}

func TestResolveReusesDirectlyDiscoveredGeneration(t *testing.T) {
	shape := &fixedShape{generations: []Generation[string]{{Token: "committed", Artifact: "artifact"}}}
	lockDir := filepath.Join(t.TempDir(), "unused-lock")
	artifact, err := Resolve(context.Background(), nil, ResolveOptions{LockDir: lockDir, LockName: "build"}, shape)
	if err != nil {
		t.Fatal(err)
	}
	if artifact != "artifact" || shape.builds != 0 {
		t.Fatalf("artifact = %q, builds = %d", artifact, shape.builds)
	}
	if _, err := os.Stat(lockDir); !os.IsNotExist(err) {
		t.Fatalf("fast-path acquired lock: %v", err)
	}
}

func TestResolveSubprocessCoalescing(t *testing.T) {
	if os.Getenv(resolveTestRoleEnv) != "" {
		runResolveTestRole(t)
		return
	}

	t.Run("non-fresh same key", func(t *testing.T) {
		root := t.TempDir()
		results := runResolveChildren(t, root, []string{"X", "X", "X"}, false)
		assertAllEqual(t, results)
		assertBuildCount(t, root, "X", 1)
	})
	t.Run("fresh same key", func(t *testing.T) {
		root := t.TempDir()
		writeStubToken(t, root, "X", "X-seed")
		results := runResolveChildren(t, root, []string{"X", "X", "X"}, true)
		assertAllEqual(t, results)
		if results[0] == "X-seed" {
			t.Fatal("fresh resolve reused pre-lock generation")
		}
		assertBuildCount(t, root, "X", 1)
	})
	t.Run("different keys share lock", func(t *testing.T) {
		root := t.TempDir()
		results := runResolveChildren(t, root, []string{"X", "Y", "X"}, false)
		if results[0] != results[2] || results[0] == results[1] {
			t.Fatalf("results = %v", results)
		}
		assertBuildCount(t, root, "X", 1)
		assertBuildCount(t, root, "Y", 1)
	})
}

type resolveChild struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
}

func runResolveChildren(t *testing.T, root string, keys []string, fresh bool) []string {
	t.Helper()
	children := make([]resolveChild, 0, len(keys))
	for _, key := range keys {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(cancel)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestResolveSubprocessCoalescing$") //nolint:gosec
		cmd.Env = append(os.Environ(),
			resolveTestRoleEnv+"=child",
			resolveTestRootEnv+"="+root,
			resolveTestKeyEnv+"="+key,
			resolveTestFreshEnv+"="+strconv.FormatBool(fresh),
		)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		children = append(children, resolveChild{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout)})
	}
	for i := range children {
		line, err := children[i].stdout.ReadString('\n')
		if err != nil || strings.TrimSpace(line) != "ready" {
			t.Fatalf("child %d readiness = %q, %v", i, line, err)
		}
	}
	for i := range children {
		if _, err := children[i].stdin.Write([]byte{1}); err != nil {
			t.Fatal(err)
		}
		if err := children[i].stdin.Close(); err != nil {
			t.Fatal(err)
		}
	}
	results := make([]string, len(children))
	for i := range children {
		line, err := children[i].stdout.ReadString('\n')
		if err != nil {
			t.Fatalf("child %d result: %v", i, err)
		}
		results[i] = strings.TrimSpace(line)
		if err := children[i].cmd.Wait(); err != nil {
			t.Fatalf("child %d failed: %v", i, err)
		}
	}
	return results
}

func runResolveTestRole(t *testing.T) {
	t.Helper()
	fresh, err := strconv.ParseBool(os.Getenv(resolveTestFreshEnv))
	if err != nil {
		t.Fatal(err)
	}
	shape := &stubShape{root: os.Getenv(resolveTestRootEnv), key: os.Getenv(resolveTestKeyEnv)}
	artifact, err := resolve(context.Background(), nil, ResolveOptions{
		LockDir:      filepath.Join(shape.root, "lock"),
		LockName:     "build",
		RequireFresh: fresh,
	}, shape, resolveHooks{afterSnapshot: func() {
		if _, err := os.Stdout.WriteString("ready\n"); err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			t.Fatal(err)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stdout.WriteString(artifact + "\n"); err != nil {
		t.Fatal(err)
	}
}

func writeStubToken(t *testing.T, root, key, token string) {
	t.Helper()
	dir := filepath.Join(root, "tokens", key)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, token), []byte(token), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertAllEqual(t *testing.T, results []string) {
	t.Helper()
	for _, result := range results[1:] {
		if result != results[0] {
			t.Fatalf("results = %v", results)
		}
	}
}

func assertBuildCount(t *testing.T, root, key string, want int) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "builds-"+key))
	if err != nil {
		t.Fatal(err)
	}
	got, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("build count for %s = %d, want %d", key, got, want)
	}
}
