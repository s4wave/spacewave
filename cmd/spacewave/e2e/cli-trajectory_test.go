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

	"github.com/aperturerobotics/fastjson"
)

const enableCLITestscriptEnv = "SPACEWAVE_CLI_TESTSCRIPT"

var updateSnapshots = flag.Bool("update", false, "refresh CLI trajectory snapshots")

type scriptState struct {
	bin      string
	repoRoot string
	work     string
	// dir is the working directory of commands, set by cd.
	dir    string
	env    []string
	stdout string
	stderr string
}

// TIER: pr
func TestSpacewaveCLITrajectoryScripts(t *testing.T) {
	// Require explicit enablement before running the CLI trajectory scripts.
	if os.Getenv(enableCLITestscriptEnv) != "true" {
		t.Skipf("set %s=true to run CLI trajectory scripts", enableCLITestscriptEnv)
	}

	// Build the CLI binary used by every trajectory script.
	repoRoot := repoRoot(t)
	bin := filepath.Join(t.TempDir(), "spacewave")
	build := exec.Command("go", "build", "-tags", "skip_e2e", "-o", bin, "./cmd/spacewave")
	build.Dir = repoRoot
	build.Env = os.Environ()
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build spacewave CLI: %v\n%s", err, out)
	}

	// Discover the CLI trajectory scripts in both supported file formats.
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

	// Run each CLI trajectory in an isolated working directory.
	for _, script := range scripts {
		t.Run(strings.TrimSuffix(filepath.Base(script), filepath.Ext(script)), func(t *testing.T) {
			// Create an isolated workspace for this CLI trajectory.
			work, err := os.MkdirTemp("", "swcli-")
			if err != nil {
				t.Fatal(err)
			}

			// Stop any daemon the script started, including one left by a
			// script that failed or never ran stop, before deleting its state.
			t.Cleanup(func() {
				stopDaemon(t, bin, filepath.Join(work, "state"))
				_ = os.RemoveAll(work)
			})

			// Execute the trajectory using the built CLI and isolated workspace.
			runScript(t, script, scriptState{
				bin:      bin,
				repoRoot: repoRoot,
				work:     work,
				dir:      work,
				env:      os.Environ(),
			})
		})
	}
}

func runScript(t *testing.T, path string, st scriptState) {
	// Read the CLI trajectory script and dispatch its command lines.
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
		case "bun-help":
			st = runBunHelp(t, path, idx+1, fields, st)
		case "cd":
			if len(fields) != 2 {
				t.Fatalf("%s:%d: usage: cd DIR", path, idx+1)
			}
			st.dir = expand(fields[1], st)
		case "cmp":
			if len(fields) != 3 {
				t.Fatalf("%s:%d: usage: cmp FILE1 FILE2", path, idx+1)
			}
			compareFiles(t, path, idx+1, expand(fields[1], st), expand(fields[2], st))
		case "env":
			if len(fields) != 2 || !strings.Contains(fields[1], "=") {
				t.Fatalf("%s:%d: usage: env KEY=VALUE", path, idx+1)
			}
			st.env = append(st.env, expand(fields[1], st))
		case "git":
			st = runGit(t, path, idx+1, fields[1:], st)
		case "git-fixture":
			if len(fields) != 2 {
				t.Fatalf("%s:%d: usage: git-fixture PATH", path, idx+1)
			}
			createGitFixture(t, path, idx+1, expand(fields[1], st))
		case "go-build":
			st = runGoBuild(t, path, idx+1, fields, st)
		case "package-script":
			assertPackageScript(t, path, idx+1, line, st)
		case "readme-command":
			assertReadmeCommand(t, path, idx+1, line, st)
		case "spacewave", "!":
			st = runCommandLine(t, path, idx+1, line, st)
		case "stdout":
			assertOutputContains(t, path, idx+1, "stdout", st.stdout, line)
		case "stderr":
			assertOutputContains(t, path, idx+1, "stderr", st.stderr, line)
		case "stdout-snapshot":
			assertOutputSnapshot(t, path, idx+1, "stdout", st.stdout, line, st)
		case "write-file":
			if len(fields) != 3 {
				t.Fatalf("%s:%d: usage: write-file PATH SIZE", path, idx+1)
			}
			writePatternFile(t, path, idx+1, expand(fields[1], st), fields[2])
		default:
			t.Fatalf("%s:%d: unknown directive %q", path, idx+1, fields[0])
		}
	}
}

func runCommandLine(t *testing.T, path string, lineNo int, line string, st scriptState) scriptState {
	// Parse the CLI command and expand its trajectory variables.
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

	// Bound the CLI process lifetime and cancel its context on return.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Run the CLI command with the trajectory directory and captured output.
	cmd := exec.CommandContext(ctx, st.bin, args[1:]...)
	cmd.Dir = st.dir
	cmd.Env = st.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	// Retain CLI output and verify the expected command result.
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

// stopDaemon stops the daemon serving statePath, if one is running.
func stopDaemon(t *testing.T, bin, statePath string) {
	// Bound the daemon stop command during trajectory cleanup.
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Stop the daemon serving the trajectory state directory.
	out, err := exec.CommandContext(ctx, bin, "stop", "--state-path", statePath).CombinedOutput()
	if err != nil {
		t.Errorf("stop daemon at %s: %v\n%s", statePath, err, out)
	}
}

// runGit runs git with args in the script's working directory.
func runGit(t *testing.T, path string, lineNo int, args []string, st scriptState) scriptState {
	// Require Git and expand its trajectory command arguments.
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("%s:%d: git executable not found: %v", path, lineNo, err)
	}
	for i := range args {
		args[i] = expand(args[i], st)
	}

	// Bound the Git process lifetime and cancel its context on return.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Run Git in the trajectory directory with captured output.
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = st.dir
	cmd.Env = st.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	// Retain Git output and verify successful completion.
	st.stdout = stdout.String()
	st.stderr = stderr.String()
	if ctx.Err() != nil {
		t.Fatalf("%s:%d: git %s timed out\nstderr:\n%s", path, lineNo, strings.Join(args, " "), st.stderr)
	}
	if err != nil {
		t.Fatalf("%s:%d: git %s: %v\nstdout:\n%s\nstderr:\n%s", path, lineNo, strings.Join(args, " "), err, st.stdout, st.stderr)
	}
	return st
}

// writePatternFile writes size deterministic bytes to filePath.
func writePatternFile(t *testing.T, path string, lineNo int, filePath, size string) {
	// Generate deterministic file contents from the trajectory size.
	t.Helper()
	n, err := strconv.Atoi(size)
	if err != nil {
		t.Fatalf("%s:%d: parse size: %v", path, lineNo, err)
	}
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(i * 7 % 251)
	}

	// Write the generated trajectory file to its requested path.
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		t.Fatalf("%s:%d: write %s: %v", path, lineNo, filePath, err)
	}
}

// compareFiles fails unless the two files have identical contents.
func compareFiles(t *testing.T, path string, lineNo int, a, b string) {
	// Read both trajectory files and verify identical contents.
	t.Helper()
	dataA, err := os.ReadFile(a)
	if err != nil {
		t.Fatalf("%s:%d: %v", path, lineNo, err)
	}
	dataB, err := os.ReadFile(b)
	if err != nil {
		t.Fatalf("%s:%d: %v", path, lineNo, err)
	}
	if !bytes.Equal(dataA, dataB) {
		t.Fatalf("%s:%d: %s (%d bytes) differs from %s (%d bytes)", path, lineNo, a, len(dataA), b, len(dataB))
	}
}

func runBunHelp(t *testing.T, path string, lineNo int, fields []string, st scriptState) scriptState {
	t.Helper()

	if len(fields) != 2 {
		t.Fatalf("%s:%d: usage: bun-help COMMAND", path, lineNo)
	}
	return runRepoCommand(t, path, lineNo, st, "bun", fields[1], "--help")
}

func runGoBuild(t *testing.T, path string, lineNo int, fields []string, st scriptState) scriptState {
	t.Helper()

	if len(fields) != 2 {
		t.Fatalf("%s:%d: usage: go-build PACKAGE", path, lineNo)
	}
	out := filepath.Join(st.work, "readme-go-build")
	return runRepoCommand(t, path, lineNo, st, "go", "build", "-o", out, fields[1])
}

func runRepoCommand(t *testing.T, path string, lineNo int, st scriptState, name string, args ...string) scriptState {
	// Bound the repository command lifetime for trajectory checks.
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Run the repository command with captured standard output and error.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = st.repoRoot
	cmd.Env = st.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	// Retain command output and require successful silent-error completion.
	st.stdout = stdout.String()
	st.stderr = stderr.String()
	if ctx.Err() != nil {
		t.Fatalf("%s:%d: %s timed out", path, lineNo, strings.Join(append([]string{name}, args...), " "))
	}
	if err != nil {
		t.Fatalf("%s:%d: %s: %v\nstdout:\n%s\nstderr:\n%s", path, lineNo, strings.Join(append([]string{name}, args...), " "), err, st.stdout, st.stderr)
	}
	if st.stderr != "" {
		t.Fatalf("%s:%d: %s wrote stderr:\n%s", path, lineNo, strings.Join(append([]string{name}, args...), " "), st.stderr)
	}
	return st
}

func createGitFixture(t *testing.T, path string, lineNo int, repoPath string) {
	// Create the source files for the trajectory Git fixture.
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("%s:%d: git executable not found: %v", path, lineNo, err)
	}
	if err := os.MkdirAll(filepath.Join(repoPath, "src"), 0o755); err != nil {
		t.Fatalf("%s:%d: create fixture repo: %v", path, lineNo, err)
	}
	writeFixtureFile(t, path, lineNo, filepath.Join(repoPath, "README.md"), "fixture repo\n")
	writeFixtureFile(t, path, lineNo, filepath.Join(repoPath, "src", "fixture.go"), "package fixture\n\nconst Name = \"spacewave\"\n")

	// Initialize the Git fixture and commit its initial source files.
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
	// Bound the Git fixture command lifetime during setup.
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Run the Git fixture command and verify its completion.
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repoPath}, args...)...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("%s:%d: git %s timed out", path, lineNo, strings.Join(args, " "))
	}
	if err != nil {
		t.Fatalf("%s:%d: git %s: %v\n%s", path, lineNo, strings.Join(args, " "), err, out)
	}
}

func assertReadmeCommand(t *testing.T, path string, lineNo int, line string, st scriptState) {
	// Read the required README command and verify its documented occurrence.
	t.Helper()
	want, err := quotedArg(line, "readme-command")
	if err != nil {
		t.Fatalf("%s:%d: %v", path, lineNo, err)
	}
	data, err := os.ReadFile(filepath.Join(st.repoRoot, "README.md"))
	if err != nil {
		t.Fatalf("%s:%d: read README.md: %v", path, lineNo, err)
	}
	if !strings.Contains(string(data), "\n"+want+"\n") {
		t.Fatalf("%s:%d: README.md missing command %q", path, lineNo, want)
	}
}

func assertPackageScript(t *testing.T, path string, lineNo int, line string, st scriptState) {
	// Read the package script expectation and parse the repository manifest.
	t.Helper()
	name, want, err := packageScriptArgs(line)
	if err != nil {
		t.Fatalf("%s:%d: %v", path, lineNo, err)
	}
	data, err := os.ReadFile(filepath.Join(st.repoRoot, "package.json"))
	if err != nil {
		t.Fatalf("%s:%d: read package.json: %v", path, lineNo, err)
	}
	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		t.Fatalf("%s:%d: parse package.json: %v", path, lineNo, err)
	}

	// Verify the repository manifest contains the expected package script.
	raw := v.GetStringBytes("scripts", name)
	if raw == nil {
		t.Fatalf("%s:%d: package.json missing script %q", path, lineNo, name)
	}
	got := string(raw)
	if got != want {
		t.Fatalf("%s:%d: package.json script %q mismatch\nwant: %s\ngot:  %s", path, lineNo, name, want, got)
	}
}

func packageScriptArgs(line string) (string, string, error) {
	// Parse the package-script directive and decode its expected command.
	raw := strings.TrimSpace(strings.TrimPrefix(line, "package-script"))
	name, quoted, ok := strings.Cut(raw, " ")
	if !ok || name == "" {
		return "", "", strconv.ErrSyntax
	}
	want, err := strconv.Unquote(strings.TrimSpace(quoted))
	return name, want, err
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
	// Parse the snapshot directive and normalize its trajectory output.
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

	// Read the saved snapshot and verify exact normalized output.
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
	// Locate the trajectory repository from this source file.
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
