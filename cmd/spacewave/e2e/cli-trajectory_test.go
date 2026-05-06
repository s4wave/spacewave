//go:build !js

package e2e_test

import (
	"bytes"
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/pkg/errors"
)

const enableCLITestscriptEnv = "SPACEWAVE_CLI_TESTSCRIPT"

var updateSnapshots = flag.Bool("update", false, "refresh CLI trajectory snapshots")

type scriptState struct {
	bin    string
	work   string
	env    []string
	stdout string
	stderr string
}

func TestSpacewaveCLITrajectoryScripts(t *testing.T) {
	if os.Getenv(enableCLITestscriptEnv) != "true" {
		t.Skipf("set %s=true to run CLI trajectory scripts", enableCLITestscriptEnv)
	}

	repoRoot := repoRoot(t)
	bin := filepath.Join(t.TempDir(), "spacewave")
	build := exec.Command("go", "build", "-tags", "skip_e2e", "-o", bin, "./cmd/spacewave")
	build.Dir = repoRoot
	build.Env = os.Environ()
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build spacewave CLI: %v\n%s", err, out)
	}

	var scripts []string
	for _, ext := range []string{"*.txt", "*.txtar"} {
		matches, err := filepath.Glob(filepath.Join(repoRoot, "cmd", "spacewave", "e2e", "testdata", "script", ext))
		if err != nil {
			t.Fatal(err)
		}
		scripts = append(scripts, matches...)
	}
	if len(scripts) == 0 {
		t.Fatal("no CLI trajectory scripts found")
	}

	for _, script := range scripts {
		t.Run(strings.TrimSuffix(filepath.Base(script), filepath.Ext(script)), func(t *testing.T) {
			work, err := os.MkdirTemp("", "swcli-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = os.RemoveAll(work)
			})
			runScript(t, script, scriptState{
				bin:  bin,
				work: work,
				env:  os.Environ(),
			})
		})
	}
}

func runScript(t *testing.T, path string, st scriptState) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	for idx, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "env":
			if len(fields) != 2 || !strings.Contains(fields[1], "=") {
				t.Fatalf("%s:%d: usage: env KEY=VALUE", path, idx+1)
			}
			st.env = append(st.env, expand(fields[1], st))
		case "git-fixture":
			if len(fields) != 2 {
				t.Fatalf("%s:%d: usage: git-fixture PATH", path, idx+1)
			}
			createGitFixture(t, path, idx+1, expand(fields[1], st))
		case "volume-stats-min":
			assertVolumeStatsMin(t, path, idx+1, fields, st)
		case "spacewave", "!":
			st = runCommandLine(t, path, idx+1, line, st)
		case "stdout":
			assertOutputContains(t, path, idx+1, "stdout", st.stdout, line)
		case "stderr":
			assertOutputContains(t, path, idx+1, "stderr", st.stderr, line)
		case "stdout-snapshot":
			assertOutputSnapshot(t, path, idx+1, "stdout", st.stdout, line, st)
		default:
			t.Fatalf("%s:%d: unknown directive %q", path, idx+1, fields[0])
		}
	}
}

func runCommandLine(t *testing.T, path string, lineNo int, line string, st scriptState) scriptState {
	t.Helper()

	wantFailure := false
	raw := line
	if strings.HasPrefix(raw, "! ") {
		wantFailure = true
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "! "))
	}
	args := strings.Fields(raw)
	if len(args) == 0 || args[0] != "spacewave" {
		t.Fatalf("%s:%d: command must start with spacewave", path, lineNo)
	}
	for i := 1; i < len(args); i++ {
		args[i] = expand(args[i], st)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, st.bin, args[1:]...)
	cmd.Dir = st.work
	cmd.Env = st.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	st.stdout = stdout.String()
	st.stderr = stderr.String()
	if ctx.Err() != nil {
		t.Fatalf("%s:%d: command timed out: %s", path, lineNo, line)
	}
	if wantFailure {
		if err == nil {
			t.Fatalf("%s:%d: command unexpectedly succeeded: %s\nstdout:\n%s\nstderr:\n%s", path, lineNo, line, st.stdout, st.stderr)
		}
		return st
	}
	if err != nil {
		t.Fatalf("%s:%d: command failed: %s: %v\nstdout:\n%s\nstderr:\n%s", path, lineNo, line, err, st.stdout, st.stderr)
	}
	return st
}

func createGitFixture(t *testing.T, path string, lineNo int, repoPath string) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("%s:%d: git executable not found: %v", path, lineNo, err)
	}
	if err := os.MkdirAll(filepath.Join(repoPath, "src"), 0o755); err != nil {
		t.Fatalf("%s:%d: create fixture repo: %v", path, lineNo, err)
	}
	writeFixtureFile(t, path, lineNo, filepath.Join(repoPath, "README.md"), "fixture repo\n")
	writeFixtureFile(t, path, lineNo, filepath.Join(repoPath, "src", "fixture.go"), "package fixture\n\nconst Name = \"spacewave\"\n")

	runGitFixtureCommand(t, path, lineNo, repoPath, "init")
	runGitFixtureCommand(t, path, lineNo, repoPath, "checkout", "-b", "main")
	runGitFixtureCommand(t, path, lineNo, repoPath, "config", "user.name", "Spacewave CLI Fixture")
	runGitFixtureCommand(t, path, lineNo, repoPath, "config", "user.email", "cli-fixture@example.test")
	runGitFixtureCommand(t, path, lineNo, repoPath, "add", ".")
	runGitFixtureCommand(t, path, lineNo, repoPath, "commit", "-m", "initial fixture")
}

func writeFixtureFile(t *testing.T, path string, lineNo int, filePath string, data string) {
	t.Helper()

	if err := os.WriteFile(filePath, []byte(data), 0o644); err != nil {
		t.Fatalf("%s:%d: write fixture file %s: %v", path, lineNo, filePath, err)
	}
}

func runGitFixtureCommand(t *testing.T, path string, lineNo int, repoPath string, args ...string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repoPath}, args...)...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("%s:%d: git %s timed out", path, lineNo, strings.Join(args, " "))
	}
	if err != nil {
		t.Fatalf("%s:%d: git %s: %v\n%s", path, lineNo, strings.Join(args, " "), err, out)
	}
}

func assertVolumeStatsMin(t *testing.T, path string, lineNo int, fields []string, st scriptState) {
	t.Helper()

	if len(fields) != 3 {
		t.Fatalf("%s:%d: usage: volume-stats-min BLOCKS BYTES", path, lineNo)
	}
	wantBlocks, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		t.Fatalf("%s:%d: parse block floor: %v", path, lineNo, err)
	}
	wantBytes, err := strconv.ParseUint(fields[2], 10, 64)
	if err != nil {
		t.Fatalf("%s:%d: parse byte floor: %v", path, lineNo, err)
	}

	blocks, bytes, err := volumeStats(st.work)
	if err != nil {
		t.Fatalf("%s:%d: read volume stats: %v", path, lineNo, err)
	}
	if blocks < wantBlocks || bytes < wantBytes {
		t.Fatalf("%s:%d: volume stats below floor: got blocks=%d bytes=%d, want at least blocks=%d bytes=%d", path, lineNo, blocks, bytes, wantBlocks, wantBytes)
	}
}

func volumeStats(work string) (uint64, uint64, error) {
	matches, err := filepath.Glob(filepath.Join(work, "state", "p_local_*.s4wave"))
	if err != nil {
		return 0, 0, err
	}
	if len(matches) != 1 {
		return 0, 0, errors.Errorf("expected one local provider volume, got %d", len(matches))
	}

	db, err := bdb.Open(matches[0], 0o444, &bdb.Options{ReadOnly: true})
	if err != nil {
		return 0, 0, err
	}
	defer db.Close()

	var blocks, bytes uint64
	err = db.View(func(tx *bdb.Tx) error {
		bucket := tx.Bucket([]byte("hydra"))
		if bucket == nil {
			return errors.New("missing hydra bucket")
		}
		return bucket.ForEach(func(_, value []byte) error {
			blocks++
			bytes += uint64(len(value))
			return nil
		})
	})
	return blocks, bytes, err
}

func assertOutputContains(t *testing.T, path string, lineNo int, name, got, line string) {
	t.Helper()

	want, err := quotedArg(line, name)
	if err != nil {
		t.Fatalf("%s:%d: %v", path, lineNo, err)
	}
	if !strings.Contains(got, want) {
		t.Fatalf("%s:%d: %s missing %q\n%s:\n%s", path, lineNo, name, want, name, got)
	}
}

func assertOutputSnapshot(t *testing.T, path string, lineNo int, name, got, line string, st scriptState) {
	t.Helper()

	rel, err := quotedArg(line, name+"-snapshot")
	if err != nil {
		t.Fatalf("%s:%d: %v", path, lineNo, err)
	}
	snapshotPath := filepath.Join(filepath.Dir(path), rel)
	got = normalizeSnapshotOutput(got, st)
	if *updateSnapshots {
		if err := os.WriteFile(snapshotPath, []byte(got), 0o644); err != nil {
			t.Fatalf("%s:%d: update snapshot: %v", path, lineNo, err)
		}
		return
	}

	want, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("%s:%d: read snapshot: %v", path, lineNo, err)
	}
	if got != string(want) {
		t.Fatalf("%s:%d: %s snapshot mismatch\nwant:\n%s\ngot:\n%s", path, lineNo, name, want, got)
	}
}

func quotedArg(line, directive string) (string, error) {
	raw := strings.TrimSpace(strings.TrimPrefix(line, directive))
	if raw == "" {
		return "", strconv.ErrSyntax
	}
	return strconv.Unquote(raw)
}

func expand(value string, st scriptState) string {
	value = strings.ReplaceAll(value, "$WORK", st.work)
	return strings.ReplaceAll(value, "$SPACEWAVE", st.bin)
}

var snapshotReplacements = []struct {
	re   *regexp.Regexp
	with string
}{
	{regexp.MustCompile(`01[0-9a-z]{6}\.\.\.`), "01xxxxxx..."},
	{regexp.MustCompile(`01[0-9a-z]{24}`), "01xxxxxxxxxxxxxxxxxxxxxxxx"},
	{regexp.MustCompile(`12D3Koo[0-9A-Za-z]{13}\.\.\.`), "12D3KooXXXXXXXXXXXXX..."},
}

func normalizeSnapshotOutput(got string, st scriptState) string {
	got = strings.ReplaceAll(got, st.work, "$WORK")
	for _, repl := range snapshotReplacements {
		got = repl.re.ReplaceAllString(got, repl.with)
	}
	return got
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}
