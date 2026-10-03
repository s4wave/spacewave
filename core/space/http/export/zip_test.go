package space_http_export

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	space_unixfs "github.com/s4wave/spacewave/core/space/unixfs"
	git_world "github.com/s4wave/spacewave/db/git/world"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

func TestExportZipGitRepoProjectionMetadata(t *testing.T) {
	// Prepare the context and logger for the repository export.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for the exported World.
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	// Start the World testbed and retain it through the export.
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Register the Git operations used to create the exported repository.
	gitOpc := world.NewLookupOpController("test-export-git-repo-projection", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	// Create an empty repository in the World.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp("repo/export", nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}

	// Open the projected filesystem for the repository Space.
	rootHandle, err := space_unixfs.BuildFSHandle(le, world.NewEngineWorldState(wtb.Engine, false), 14, "space-export")
	if err != nil {
		t.Fatal(err)
	}
	defer rootHandle.Release()

	// Open the repository metadata projection for export.
	repoHandle, _, err := rootHandle.LookupPath(ctx, "u/14/so/space-export/-/repo/export/-")
	if err != nil {
		t.Fatal(err)
	}
	defer repoHandle.Release()

	// Export the repository projection into an in-memory archive.
	var buf bytes.Buffer
	if err := exportZip(ctx, &buf, repoHandle); err != nil {
		t.Fatal(err)
	}

	// Find the repository HEAD in the exported archive.
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var head *zip.File
	for _, f := range zr.File {
		if f.Name == "HEAD" {
			head = f
			break
		}
	}
	if head == nil {
		t.Fatal("missing HEAD in git repo projection export")
	}

	// Read the exported HEAD and close its archive stream.
	rc, err := head.Open()
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(rc)
	if closeErr := rc.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}

	// Verify that HEAD identifies the default branch.
	if string(content) != "ref: refs/heads/master\n" {
		t.Fatalf("unexpected exported HEAD content %q", string(content))
	}
}

func TestExportZipPopulatedGitRepoProjectionUsesMetadata(t *testing.T) {
	// Prepare the context and logger for the populated repository export.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for the exported World.
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	// Start the World testbed and retain it through the export.
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Register the Git operations used to populate the exported repository.
	gitOpc := world.NewLookupOpController("test-export-populated-git-repo-projection", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	// Create the repository and commit a README in its World state.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp("repo/export-populated", nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}
	workdir := memfs.New()
	var commitHash string
	_, _, err = git_world.AccessWorldObjectRepo(ctx, ws, "repo/export-populated", true, nil, workdir, nil, func(repo *git.Repository) error {
		hash, err := commitExportReadme(repo, workdir)
		if err != nil {
			return err
		}
		commitHash = hash
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Open the projected filesystem for the repository Space.
	rootHandle, err := space_unixfs.BuildFSHandle(le, world.NewEngineWorldState(wtb.Engine, false), 15, "space-export-populated")
	if err != nil {
		t.Fatal(err)
	}
	defer rootHandle.Release()

	// Open the populated repository metadata projection for export.
	repoHandle, _, err := rootHandle.LookupPath(ctx, "u/15/so/space-export-populated/-/repo/export-populated/-")
	if err != nil {
		t.Fatal(err)
	}
	defer repoHandle.Release()

	// Export the repository projection into an in-memory archive.
	var buf bytes.Buffer
	if err := exportZip(ctx, &buf, repoHandle); err != nil {
		t.Fatal(err)
	}

	// Index the archive entries and exclude the repository source tree.
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	entries := make(map[string]*zip.File)
	for _, f := range zr.File {
		entries[f.Name] = f
	}
	if _, ok := entries["README.md"]; ok {
		t.Fatal("exported source-tree README from git/repo projection")
	}

	// Verify that the exported HEAD identifies the default branch.
	head := readZipFile(t, entries, "HEAD")
	if head != "ref: refs/heads/master\n" {
		t.Fatalf("unexpected exported HEAD content %q", head)
	}

	// Verify that the exported branch identifies the README commit.
	ref := readZipFile(t, entries, "refs/heads/master")
	if ref != commitHash+"\n" {
		t.Fatalf("unexpected exported branch content %q, want %q", ref, commitHash+"\n")
	}
}

func commitExportReadme(repo *git.Repository, workdir billy.Filesystem) (string, error) {
	// Open the repository worktree for the README fixture.
	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}

	// Create and write the README fixture in the worktree.
	f, err := workdir.Create("README.md")
	if err != nil {
		return "", err
	}
	if _, err := f.Write([]byte("# Exported\n")); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}

	// Stage the README fixture for the repository commit.
	if _, err := wt.Add("README.md"); err != nil {
		return "", err
	}

	// Commit the README fixture with its author and committer.
	sig := &object.Signature{
		Name:  "Test",
		Email: "test@example.com",
		When:  time.Now(),
	}
	hash, err := wt.Commit("initial commit", &git.CommitOptions{
		Author:    sig,
		Committer: sig,
	})
	if err != nil {
		return "", err
	}
	return hash.String(), nil
}

func readZipFile(t *testing.T, entries map[string]*zip.File, name string) string {
	// Require the named entry in the exported archive.
	t.Helper()
	f, ok := entries[name]
	if !ok {
		t.Fatalf("missing zip entry %s", name)
	}

	// Read the archive entry and close its stream.
	rc, err := f.Open()
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(rc)
	if closeErr := rc.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
