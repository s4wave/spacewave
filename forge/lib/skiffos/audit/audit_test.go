package forge_lib_skiffos_audit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
)

// TestAudit checks the audit's readings on a SkiffOS repository whose head
// merges a side branch: the commits behind count both branches, as git
// rev-list does, a commit upstream lacks is diverged, and each commit and
// Buildroot pin is described by its nearest tag of the right kind.
func TestAudit(t *testing.T) {
	// Build a Buildroot fork with a skiff tag one commit below its head, and
	// an unrelated tag the audit must ignore.
	tmp := t.TempDir()
	br, up := filepath.Join(tmp, "buildroot"), filepath.Join(tmp, "skiffos")
	runGit(t, tmp, "init", "-q", br)
	b1 := commit(t, br, "1")
	runGit(t, br, "tag", "-a", "-m", "release", "skiff-1")
	b2 := commit(t, br, "2")
	runGit(t, br, "tag", "settings-v1")

	// Build SkiffOS: s1 pins b1 and is released, s2 pins b2, a side branch
	// from s1 adds x, and the head merges it.
	runGit(t, tmp, "init", "-q", "-b", "master", up)
	s1 := pin(t, up, b1)
	runGit(t, up, "tag", "2026.01")
	s2 := pin(t, up, b2)
	runGit(t, up, "checkout", "-q", "-b", "side", s1)
	commit(t, up, "x")
	runGit(t, up, "checkout", "-q", "master")
	runGit(t, up, "merge", "-q", "--no-ff", "-m", "merge", "side")
	head := runGit(t, up, "rev-parse", "HEAD")
	upRepo, brRepo := open(t, up), open(t, br)

	// Relate each build commit to the head.
	for _, c := range []struct {
		build string
		want  isReport_Freshness
	}{
		{head, &Report_Current{Current: &Current{}}},
		{s2, &Report_Behind{Behind: &Behind{Commits: 2}}},
		{s1, &Report_Behind{Behind: &Behind{Commits: 3}}},
		{b1, &Report_Diverged{Diverged: &Diverged{}}},
	} {
		got, err := readFreshness(upRepo, plumbing.NewHash(c.build), plumbing.NewHash(head))
		if err != nil {
			t.Fatal(err)
		}
		if gotRep, wantRep := (&Report{Freshness: got}), (&Report{Freshness: c.want}); !gotRep.EqualVT(wantRep) {
			t.Fatalf("build %s: %s, want %s", c.build, gotRep.MarshalProtoText(), wantRep.MarshalProtoText())
		}
	}

	// Describe s2 and the head's pin, b2, each by its own kind of tag.
	rev, err := readRevision(upRepo, plumbing.NewHash(s2))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := describeAll(upRepo, releaseTagPrefix, rev.Skiffos); err != nil {
		t.Fatal(err)
	}
	count, err := describeAll(brRepo, skiffTagPrefix, rev.Buildroot)
	if err != nil {
		t.Fatal(err)
	}
	if d := rev.Skiffos; d.Tag != "2026.01" || d.Distance != 1 {
		t.Fatalf("SkiffOS described as %s~%d", d.Tag, d.Distance)
	}
	if d := rev.Buildroot; d.Commit != b2 || d.Tag != "skiff-1" || d.Distance != 1 {
		t.Fatalf("Buildroot %s described as %s~%d", d.Commit, d.Tag, d.Distance)
	}
	if count != 1 {
		t.Fatalf("counted %d skiff tags, want 1", count)
	}
}

// pin commits a buildroot gitlink at hash in the repository at dir.
func pin(t *testing.T, dir, hash string) string {
	t.Helper()
	runGit(t, dir, "update-index", "--add", "--cacheinfo", "160000,"+hash+",buildroot")
	runGit(t, dir, "commit", "-qm", "pin "+hash)
	return runGit(t, dir, "rev-parse", "HEAD")
}

// commit writes content to a file of that name in the repository at dir and
// commits it.
func commit(t *testing.T, dir, content string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, content), []byte(content+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", content)
	runGit(t, dir, "commit", "-qm", content)
	return runGit(t, dir, "rev-parse", "HEAD")
}

// open opens the repository at dir with go-git.
func open(t *testing.T, dir string) *git.Repository {
	t.Helper()
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

// runGit runs git in dir with a fixed identity and returns its trimmed output.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_GLOBAL=/dev/null",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
