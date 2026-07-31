// Package githooks_test exercises the repository git hooks that core.hooksPath
// installs. The hooks are shell, so nothing else compiles them and nothing else
// would notice when one stops rejecting what it exists to reject.
package githooks_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCommitMsgHookAcceptsSignedOffMessage(t *testing.T) {
	stdout, stderr, err := runCommitMsgHook(t, `fix(hook): accept a signed-off message

The body explains why the change is needed.

Signed-off-by: Hook Author <hook-author@example.com>
`)
	if err != nil {
		t.Fatalf("hook rejected a signed-off message: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
}

func TestCommitMsgHookRejectsMissingSignOff(t *testing.T) {
	_, stderr, err := runCommitMsgHook(t, `fix(hook): forget the sign-off

The body explains why the change is needed.
`)
	if err == nil {
		t.Fatal("hook accepted a message with no sign-off trailer")
	}
	if !strings.Contains(stderr, "no Developer Certificate of Origin sign-off") {
		t.Fatalf("sign-off rejection did not name its reason: %s", stderr)
	}
}

func TestCommitMsgHookRejectsSignOffWithoutAnEmail(t *testing.T) {
	// The DCO attests to an identity someone can be reached at, so a bare name
	// signs nothing.
	_, stderr, err := runCommitMsgHook(t, `fix(hook): sign off with a bare name

The body explains why the change is needed.

Signed-off-by: Hook Author
`)
	if err == nil {
		t.Fatal("hook accepted a sign-off carrying no email address")
	}
	if !strings.Contains(stderr, "no Developer Certificate of Origin sign-off") {
		t.Fatalf("hook rejected the message for some other reason: %s", stderr)
	}
}

func TestCommitMsgHookRejectsSignOffWithoutAName(t *testing.T) {
	_, stderr, err := runCommitMsgHook(t, `fix(hook): sign off with a bare address

The body explains why the change is needed.

Signed-off-by: <hook-author@example.com>
`)
	if err == nil {
		t.Fatal("hook accepted a sign-off carrying no name")
	}
	if !strings.Contains(stderr, "no Developer Certificate of Origin sign-off") {
		t.Fatalf("hook rejected the message for some other reason: %s", stderr)
	}
}

func TestCommitMsgHookRejectsSignOffQuotedInProse(t *testing.T) {
	// git interpret-trailers reads the final paragraph, so a sign-off narrated
	// mid-body is prose. Every line here stays inside the column limit, so only
	// the sign-off rule can decide the outcome.
	_, stderr, err := runCommitMsgHook(t, `fix(hook): narrate a sign-off

The commit that introduced this carried:

Signed-off-by: Hook Author <hook-author@example.com>

Quoting that line is not the same as signing off.
`)
	if err == nil {
		t.Fatal("hook accepted a sign-off quoted inside the prose")
	}
	if !strings.Contains(stderr, "no Developer Certificate of Origin sign-off") {
		t.Fatalf("hook rejected the message for some other reason: %s", stderr)
	}
}

func TestCommitMsgHookIgnoresCommentsAfterTheTrailers(t *testing.T) {
	// git appends its status commentary after the message, and the scissors
	// block carries the whole diff under --verbose. Either one read as the final
	// paragraph hides the trailer block from the parser.
	stdout, stderr, err := runCommitMsgHook(t, `fix(hook): sign off ahead of git's commentary

The body explains why the change is needed.

Signed-off-by: Hook Author <hook-author@example.com>

# Please enter the commit message for your changes. Lines starting
# with '#' will be ignored, and an empty message aborts the commit.
# ------------------------ >8 ------------------------
# Do not modify or remove the line above.
diff --git a/a b/a
`)
	if err != nil {
		t.Fatalf("hook rejected a signed-off message: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
}

func TestCommitMsgHookRejectsLowercaseSignOff(t *testing.T) {
	// --parse implies --unfold and canonicalizes the trailer it returns, so a
	// lowercase sign-off looks correct by the time the trailer loop sees it. The
	// DCO workflow greps the raw message case-sensitively and rejects it after a
	// push, which is the round trip this hook exists to spare the author.
	_, stderr, err := runCommitMsgHook(t, `fix(hook): sign off in lowercase

The body explains why the change is needed.

signed-off-by: Hook Author <hook-author@example.com>
`)
	if err == nil {
		t.Fatal("hook accepted a sign-off the DCO workflow rejects")
	}
	if !strings.Contains(stderr, "not written the way the DCO check reads it") {
		t.Fatalf("syntax rejection did not name its reason: %s", stderr)
	}
}

func TestCommitMsgHookRejectsFoldedSignOff(t *testing.T) {
	// A folded trailer is one logical trailer to git and two lines to the
	// workflow's line-oriented grep, so it passes here and fails there.
	_, stderr, err := runCommitMsgHook(t, `fix(hook): fold the sign-off

The body explains why the change is needed.

Signed-off-by:
  Hook Author <hook-author@example.com>
`)
	if err == nil {
		t.Fatal("hook accepted a folded sign-off the DCO workflow rejects")
	}
	if !strings.Contains(stderr, "not written the way the DCO check reads it") {
		t.Fatalf("syntax rejection did not name its reason: %s", stderr)
	}
}

func TestCommitMsgHookRejectsOverWideProse(t *testing.T) {
	_, stderr, err := runCommitMsgHook(t, `fix(hook): keep rejecting wide prose

`+strings.Repeat("w", 81)+`

Signed-off-by: Hook Author <hook-author@example.com>
`)
	if err == nil {
		t.Fatal("hook accepted prose wider than the column limit")
	}
	if !strings.Contains(stderr, "exceeds 80 columns") {
		t.Fatalf("width rejection did not name its reason: %s", stderr)
	}
}

func TestCommitMsgHookReportsWidthAndSignOffTogether(t *testing.T) {
	// Reporting one rule at a time costs a whole commit round trip per rule, so
	// a message that breaks both has to come back naming both.
	_, stderr, err := runCommitMsgHook(t, `fix(hook): break both rules at once

`+strings.Repeat("w", 81)+`
`)
	if err == nil {
		t.Fatal("hook accepted a message that is both wide and unsigned")
	}
	if !strings.Contains(stderr, "exceeds 80 columns") ||
		!strings.Contains(stderr, "no Developer Certificate of Origin sign-off") {
		t.Fatalf("hook reported only one of the two broken rules: %s", stderr)
	}
}

func TestCommitMsgHookAcceptsSignOffUnderAnotherTrailerSeparator(t *testing.T) {
	// git commit -s writes a colon whatever trailer.separators says, so a hook
	// that honored the setting would reject every signed commit by whoever set
	// it, including the retries its own error message asks for.
	stdout, stderr, err := runCommitMsgHook(t, `fix(hook): sign off under another separator

The body explains why the change is needed.

Signed-off-by: Hook Author <hook-author@example.com>
`, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=trailer.separators", "GIT_CONFIG_VALUE_0==")
	if err != nil {
		t.Fatalf("hook rejected a signed-off message: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
}

// runCommitMsgHook writes message to a file and runs the hook against it from a
// throwaway repository, so the hook reads the fixture and not whatever the tree
// running the tests happens to hold.
func runCommitMsgHook(t *testing.T, message string, env ...string) (string, string, error) {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "--quiet")

	path := filepath.Join(repo, "COMMIT_EDITMSG")
	if err := os.WriteFile(path, []byte(message), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(commitMsgHookPath(t), path)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

func commitMsgHookPath(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	path := filepath.Join(filepath.Dir(currentFile), "..", "..", ".githooks", "commit-msg")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	return path
}
