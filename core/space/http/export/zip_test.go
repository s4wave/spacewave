package space_http_export

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	space_unixfs "github.com/s4wave/spacewave/core/space/unixfs"
	git_world "github.com/s4wave/spacewave/db/git/world"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

func TestWalkAndZipUnixFS(t *testing.T) {
	ctx := context.Background()

	// Build an in-memory filesystem tree.
	mfs := fstest.MapFS{
		"hello.txt":         {Data: []byte("hello world")},
		"subdir/nested.txt": {Data: []byte("nested content")},
		"subdir/deep/a.txt": {Data: []byte("deep file")},
		"empty-dir":         {Mode: 0o755 | fs.ModeDir},
	}

	// Create FSCursor from io/fs.FS.
	cursor, err := unixfs_iofs.NewFSCursor(mfs)
	if err != nil {
		t.Fatal(err)
	}

	// Create FSHandle from cursor.
	fsh, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		cursor.Release()
		t.Fatal(err)
	}
	defer fsh.Release()

	// Export to zip.
	var buf bytes.Buffer
	if err := exportZip(ctx, &buf, fsh); err != nil {
		t.Fatal(err)
	}

	// Read the zip and verify contents.
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}

	// Collect entries.
	entries := make(map[string]*zip.File)
	for _, f := range zr.File {
		entries[f.Name] = f
	}

	// Verify expected files exist.
	expectedFiles := map[string]string{
		"hello.txt":         "hello world",
		"subdir/nested.txt": "nested content",
		"subdir/deep/a.txt": "deep file",
	}
	for name, wantContent := range expectedFiles {
		f, ok := entries[name]
		if !ok {
			t.Errorf("missing expected file: %s", name)
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Errorf("open %s: %v", name, err)
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Errorf("read %s: %v", name, err)
			continue
		}
		if string(data) != wantContent {
			t.Errorf("%s: got %q, want %q", name, data, wantContent)
		}
	}

	// Verify directories have trailing slash.
	expectedDirs := []string{"subdir/", "subdir/deep/"}
	for _, name := range expectedDirs {
		if _, ok := entries[name]; !ok {
			t.Errorf("missing expected directory entry: %s", name)
		}
	}

	t.Logf("zip contains %d entries", len(zr.File))
}

func TestExportZipGitRepoProjectionMetadata(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	gitOpc := world.NewLookupOpController("test-export-git-repo-projection", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp("repo/export", nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}

	rootHandle, err := space_unixfs.BuildFSHandle(le, world.NewEngineWorldState(wtb.Engine, false), 14, "space-export")
	if err != nil {
		t.Fatal(err)
	}
	defer rootHandle.Release()

	repoHandle, _, err := rootHandle.LookupPath(ctx, "u/14/so/space-export/-/repo/export/-")
	if err != nil {
		t.Fatal(err)
	}
	defer repoHandle.Release()

	var buf bytes.Buffer
	if err := exportZip(ctx, &buf, repoHandle); err != nil {
		t.Fatal(err)
	}

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
	if string(content) != "ref: refs/heads/master\n" {
		t.Fatalf("unexpected exported HEAD content %q", string(content))
	}
}

func TestExportZipPopulatedGitRepoProjectionUsesMetadata(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	gitOpc := world.NewLookupOpController("test-export-populated-git-repo-projection", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

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

	rootHandle, err := space_unixfs.BuildFSHandle(le, world.NewEngineWorldState(wtb.Engine, false), 15, "space-export-populated")
	if err != nil {
		t.Fatal(err)
	}
	defer rootHandle.Release()

	repoHandle, _, err := rootHandle.LookupPath(ctx, "u/15/so/space-export-populated/-/repo/export-populated/-")
	if err != nil {
		t.Fatal(err)
	}
	defer repoHandle.Release()

	var buf bytes.Buffer
	if err := exportZip(ctx, &buf, repoHandle); err != nil {
		t.Fatal(err)
	}

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
	head := readZipFile(t, entries, "HEAD")
	if head != "ref: refs/heads/master\n" {
		t.Fatalf("unexpected exported HEAD content %q", head)
	}
	ref := readZipFile(t, entries, "refs/heads/master")
	if ref != commitHash+"\n" {
		t.Fatalf("unexpected exported branch content %q, want %q", ref, commitHash+"\n")
	}
}

func commitExportReadme(repo *git.Repository, workdir billy.Filesystem) (string, error) {
	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}
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
	if _, err := wt.Add("README.md"); err != nil {
		return "", err
	}
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
	t.Helper()
	f, ok := entries[name]
	if !ok {
		t.Fatalf("missing zip entry %s", name)
	}
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
