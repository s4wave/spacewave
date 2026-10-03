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
	// Read the saved artifact tokens for this stub content key.
	entries, err := os.ReadDir(filepath.Join(s.root, "tokens", s.key))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Expose each saved token as an artifact generation.
	generations := make([]Generation[string], 0, len(entries))
	for _, entry := range entries {
		generations = append(generations, Generation[string]{Token: entry.Name(), Artifact: entry.Name()})
	}

	return generations, nil
}

func (s *stubShape) Build(context.Context, string) (Generation[string], error) {
	// Read the previous build count for this stub artifact key.
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

	// Record the next build number for this artifact key.
	count++
	if err := os.WriteFile(countPath, []byte(strconv.Itoa(count)+"\n"), 0o644); err != nil {
		return Generation[string]{}, err
	}

	// Save a generation token identifying the newly built stub artifact.
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
	lookups     [][]Generation[string]
	lookup      int
	builds      int
}

func (s *fixedShape) ContentKey(context.Context) (string, error) { return "key", nil }

func (s *fixedShape) Lookup(context.Context, string) ([]Generation[string], error) {
	// Reuse fixed generations when the shape has no scripted lookup sequence.
	if len(s.lookups) == 0 {
		return s.generations, nil
	}

	// Advance the scripted artifact lookup and retain its last snapshot.
	lookup := s.lookup
	if lookup >= len(s.lookups) {
		lookup = len(s.lookups) - 1
	}
	s.lookup++

	return s.lookups[lookup], nil
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

func TestResolveUsesLookupPreferenceOrder(t *testing.T) {
	// Prepare artifact generations in an order that differs from token sorting.
	shape := &fixedShape{generations: []Generation[string]{
		{Token: "z-current", Artifact: "current"},
		{Token: "a-older", Artifact: "older"},
	}}
	lockDir := filepath.Join(t.TempDir(), "unused-lock")

	// Resolve the preferred artifact without requesting a fresh generation.
	artifact, err := Resolve(context.Background(), nil, ResolveOptions{LockDir: lockDir, LockName: "build"}, shape)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that resolution reuses the preferred artifact without a build or lock.
	if artifact != "current" || shape.builds != 0 {
		t.Fatalf("artifact = %q, builds = %d", artifact, shape.builds)
	}
	if _, err := os.Stat(lockDir); !os.IsNotExist(err) {
		t.Fatalf("fast-path acquired lock: %v", err)
	}
}

func TestResolveFreshExcludesEveryPreLockGeneration(t *testing.T) {
	shape := &fixedShape{lookups: [][]Generation[string]{
		{
			{Token: "old-current", Artifact: "old-current"},
			{Token: "old-fallback", Artifact: "old-fallback"},
		},
		{
			{Token: "old-current", Artifact: "old-current"},
			{Token: "old-fallback", Artifact: "old-fallback"},
			{Token: "fresh-unpointed", Artifact: "fresh-unpointed"},
		},
	}}
	artifact, err := Resolve(
		context.Background(),
		nil,
		ResolveOptions{LockDir: t.TempDir(), LockName: "build", RequireFresh: true},
		shape,
	)
	if err != nil {
		t.Fatal(err)
	}
	if artifact != "fresh-unpointed" || shape.builds != 0 {
		t.Fatalf("artifact = %q, builds = %d", artifact, shape.builds)
	}
}

func TestResolveReusesCrashRecoveredFallback(t *testing.T) {
	shape := &fixedShape{generations: []Generation[string]{
		{Token: "a-recovered", Artifact: "recovered"},
		{Token: "z-older", Artifact: "older"},
	}}
	artifact, err := Resolve(
		context.Background(),
		nil,
		ResolveOptions{LockDir: t.TempDir(), LockName: "build"},
		shape,
	)
	if err != nil {
		t.Fatal(err)
	}
	if artifact != "recovered" || shape.builds != 0 {
		t.Fatalf("artifact = %q, builds = %d", artifact, shape.builds)
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
		// Seed an existing artifact generation before fresh subprocess resolution.
		root := t.TempDir()
		writeStubToken(t, root, "X", "X-seed")

		// Resolve the same artifact key concurrently with freshness required.
		results := runResolveChildren(t, root, []string{"X", "X", "X"}, true)

		// Verify that fresh callers share one new artifact generation.
		assertAllEqual(t, results)
		if results[0] == "X-seed" {
			t.Fatal("fresh resolve reused pre-lock generation")
		}
		assertBuildCount(t, root, "X", 1)
	})
	t.Run("different keys share lock", func(t *testing.T) {
		// Prepare concurrent artifact keys that share a build lock.
		root := t.TempDir()

		// Resolve two artifact keys through the same lock directory.
		results := runResolveChildren(t, root, []string{"X", "Y", "X"}, false)

		// Verify that only callers with the same key share an artifact generation.
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
	// Start one artifact resolver subprocess for each requested content key.
	t.Helper()
	children := make([]resolveChild, 0, len(keys))
	for _, key := range keys {
		// Configure a bounded artifact resolver subprocess for this key.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(cancel)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestResolveSubprocessCoalescing$") //nolint:gosec
		cmd.Env = append(os.Environ(),
			resolveTestRoleEnv+"=child",
			resolveTestRootEnv+"="+root,
			resolveTestKeyEnv+"="+key,
			resolveTestFreshEnv+"="+strconv.FormatBool(fresh),
		)

		// Connect the resolver subprocess input and output pipes.
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}

		// Start the resolver subprocess and retain its communication handles.
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		children = append(children, resolveChild{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout)})
	}

	// Require every resolver to finish its pre-lock artifact snapshot.
	for i := range children {
		line, err := children[i].stdout.ReadString('\n')
		if err != nil || strings.TrimSpace(line) != "ready" {
			t.Fatalf("child %d readiness = %q, %v", i, line, err)
		}
	}

	// Release every resolver to compete for the shared build lock.
	for i := range children {
		if _, err := children[i].stdin.Write([]byte{1}); err != nil {
			t.Fatal(err)
		}
		if err := children[i].stdin.Close(); err != nil {
			t.Fatal(err)
		}
	}

	// Collect the resolved artifact from each completed subprocess.
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
	// Read the artifact freshness request for this resolver subprocess.
	t.Helper()
	fresh, err := strconv.ParseBool(os.Getenv(resolveTestFreshEnv))
	if err != nil {
		t.Fatal(err)
	}

	// Resolve a stub artifact after synchronizing its pre-lock snapshot with the parent.
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

	// Send the resolved artifact token to the parent process.
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
	// Read the persisted build count for the artifact key.
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "builds-"+key))
	if err != nil {
		t.Fatal(err)
	}

	// Parse and verify the artifact build count against the expected total.
	got, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("build count for %s = %d, want %d", key, got, want)
	}
}
