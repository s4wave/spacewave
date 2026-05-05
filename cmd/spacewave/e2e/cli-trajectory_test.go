//go:build !js

package e2e_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const enableCLITestscriptEnv = "SPACEWAVE_CLI_TESTSCRIPT"

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

	scripts, err := filepath.Glob(filepath.Join(repoRoot, "cmd", "spacewave", "e2e", "testdata", "script", "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(scripts) == 0 {
		t.Fatal("no CLI trajectory scripts found")
	}

	for _, script := range scripts {
		t.Run(strings.TrimSuffix(filepath.Base(script), ".txt"), func(t *testing.T) {
			runScript(t, script, scriptState{
				bin:  bin,
				work: t.TempDir(),
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
		case "spacewave", "!":
			st = runCommandLine(t, path, idx+1, line, st)
		case "stdout":
			assertOutputContains(t, path, idx+1, "stdout", st.stdout, line)
		case "stderr":
			assertOutputContains(t, path, idx+1, "stderr", st.stderr, line)
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
