package space_unixfs

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/bucket"
	git_world "github.com/s4wave/spacewave/db/git/world"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_git "github.com/s4wave/spacewave/sdk/git"
	git_repofs "github.com/s4wave/spacewave/sdk/git/repofs"
	resource_git "github.com/s4wave/spacewave/sdk/git/resource"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
	"github.com/sirupsen/logrus"
)

type testGitWatchStatusStream struct {
	srpc.Stream
	ctx  context.Context
	msgs chan *s4wave_git.WatchStatusResponse
}

func newTestGitWatchStatusStream(ctx context.Context) *testGitWatchStatusStream {
	return &testGitWatchStatusStream{
		ctx:  ctx,
		msgs: make(chan *s4wave_git.WatchStatusResponse, 4),
	}
}

func (m *testGitWatchStatusStream) Context() context.Context {
	return m.ctx
}

func (m *testGitWatchStatusStream) Send(resp *s4wave_git.WatchStatusResponse) error {
	select {
	case m.msgs <- resp:
		return nil
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
}

func (m *testGitWatchStatusStream) SendAndClose(resp *s4wave_git.WatchStatusResponse) error {
	return m.Send(resp)
}

func (m *testGitWatchStatusStream) MsgRecv(_ srpc.Message) error {
	return nil
}

func (m *testGitWatchStatusStream) MsgSend(_ srpc.Message) error {
	return nil
}

func (m *testGitWatchStatusStream) CloseSend() error {
	return nil
}

func (m *testGitWatchStatusStream) Close() error {
	return nil
}

func TestFSCursorProjectsUnixFSObjectPaths(t *testing.T) {
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

	opc := world.NewLookupOpController("test-space-projection", wtb.EngineID, unixfs_world.LookupFsOp)
	if _, err := wtb.Bus.AddController(ctx, opc, nil); err != nil {
		t.Fatal(err)
	}

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	objectKey := "docs/demo"
	if _, _, err := unixfs_world.FsInit(
		ctx,
		ws,
		sender,
		objectKey,
		unixfs_world.FSType_FSType_FS_NODE,
		nil,
		true,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}

	tx, err := wtb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	objectCursor, _ := unixfs_world.NewFSCursorWithWriter(
		ctx,
		le,
		tx,
		objectKey,
		unixfs_world.FSType_FSType_FS_NODE,
		sender,
	)
	if err != nil {
		t.Fatal(err)
	}
	objectHandle, err := unixfs.NewFSHandle(objectCursor)
	if err != nil {
		objectCursor.Release()
		t.Fatal(err)
	}
	defer objectHandle.Release()

	if err := objectHandle.MkdirAll(ctx, []string{"nested"}, 0o755, time.Now()); err != nil {
		t.Fatal(err)
	}
	nestedHandle, _, err := objectHandle.LookupPath(ctx, "nested")
	if err != nil {
		t.Fatal(err)
	}
	if err := nestedHandle.Mknod(ctx, true, []string{"hello.txt"}, unixfs.NewFSCursorNodeType_File(), 0o644, time.Now()); err != nil {
		nestedHandle.Release()
		t.Fatal(err)
	}
	nestedHandle.Release()
	fileHandle, _, err := objectHandle.LookupPath(ctx, "nested/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := fileHandle.WriteAt(ctx, 0, []byte("hello world"), time.Now()); err != nil {
		fileHandle.Release()
		t.Fatal(err)
	}
	fileHandle.Release()
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	rootCursor := NewFSCursor(le, world.NewEngineWorldState(wtb.Engine, false), 7, "space-1")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	projectedFile, _, err := rootHandle.LookupPath(ctx, "u/7/so/space-1/-/docs/demo/-/nested/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer projectedFile.Release()

	buf := make([]byte, 32)
	n, err := projectedFile.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "hello world" {
		t.Fatalf("got %q, want %q", got, "hello world")
	}
}

func TestFSCursorDisambiguatesObjectKeyAndDescendantPaths(t *testing.T) {
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

	opc := world.NewLookupOpController("test-space-projection-overlap", wtb.EngineID, unixfs_world.LookupFsOp)
	if _, err := wtb.Bus.AddController(ctx, opc, nil); err != nil {
		t.Fatal(err)
	}

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	now := time.Now()

	writeObjectFile := func(objectKey, name, content string) {
		if _, _, err := unixfs_world.FsInit(
			ctx,
			ws,
			sender,
			objectKey,
			unixfs_world.FSType_FSType_FS_NODE,
			nil,
			true,
			now,
		); err != nil {
			t.Fatal(err)
		}

		tx, err := wtb.Engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()

		cursor, _ := unixfs_world.NewFSCursorWithWriter(
			ctx,
			le,
			tx,
			objectKey,
			unixfs_world.FSType_FSType_FS_NODE,
			sender,
		)
		handle, err := unixfs.NewFSHandle(cursor)
		if err != nil {
			cursor.Release()
			t.Fatal(err)
		}
		defer handle.Release()

		if err := handle.Mknod(ctx, true, []string{name}, unixfs.NewFSCursorNodeType_File(), 0o644, now); err != nil {
			t.Fatal(err)
		}
		fileHandle, _, err := handle.LookupPath(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		if err := fileHandle.WriteAt(ctx, 0, []byte(content), now); err != nil {
			fileHandle.Release()
			t.Fatal(err)
		}
		fileHandle.Release()
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	writeObjectFile("foo/bar", "hello.txt", "object one")
	writeObjectFile("foo/bar/files", "root.txt", "object two")

	rootCursor := NewFSCursor(le, world.NewEngineWorldState(wtb.Engine, false), 9, "space-9")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	direntNames := make([]string, 0, 2)
	fooBarHandle, _, err := rootHandle.LookupPath(ctx, "u/9/so/space-9/-/foo/bar")
	if err != nil {
		t.Fatal(err)
	}
	if err := fooBarHandle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		direntNames = append(direntNames, ent.GetName())
		return nil
	}); err != nil {
		fooBarHandle.Release()
		t.Fatal(err)
	}
	fooBarHandle.Release()

	if len(direntNames) != 2 || direntNames[0] != "-" || direntNames[1] != "files" {
		t.Fatalf("unexpected foo/bar projection children: %#v", direntNames)
	}

	firstHandle, _, err := rootHandle.LookupPath(ctx, "u/9/so/space-9/-/foo/bar/-/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer firstHandle.Release()

	secondHandle, _, err := rootHandle.LookupPath(ctx, "u/9/so/space-9/-/foo/bar/files/-/root.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer secondHandle.Release()

	buf := make([]byte, 32)
	n, err := firstHandle.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "object one" {
		t.Fatalf("got first %q, want %q", got, "object one")
	}

	n, err = secondHandle.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "object two" {
		t.Fatalf("got second %q, want %q", got, "object two")
	}
}

func TestFSCursorProjectsGitRepoMetadataPaths(t *testing.T) {
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

	gitOpc := world.NewLookupOpController("test-space-projection-git-repo", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp("repo/demo", nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}

	rootCursor := NewFSCursor(le, world.NewEngineWorldState(wtb.Engine, false), 10, "space-git-repo")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	repoHandle, _, err := rootHandle.LookupPath(ctx, "u/10/so/space-git-repo/-/repo/demo/-")
	if err != nil {
		t.Fatal(err)
	}
	defer repoHandle.Release()

	var names []string
	if err := repoHandle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		names = append(names, ent.GetName())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	expected := map[string]struct{}{
		"HEAD":        {},
		"config":      {},
		"description": {},
		"hooks":       {},
		"info":        {},
		"objects":     {},
		"refs":        {},
	}
	for _, name := range names {
		delete(expected, name)
	}
	for name := range expected {
		t.Fatalf("missing repository metadata entry %q in %v", name, names)
	}

	headHandle, _, err := rootHandle.LookupPath(ctx, "u/10/so/space-git-repo/-/repo/demo/-/HEAD")
	if err != nil {
		t.Fatal(err)
	}
	defer headHandle.Release()
	buf := make([]byte, 64)
	n, err := headHandle.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "ref: refs/heads/master\n" {
		t.Fatalf("got HEAD %q, want %q", got, "ref: refs/heads/master\n")
	}
}

func TestFSCursorRejectsInvalidGitRepoProjection(t *testing.T) {
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

	ws := world.NewEngineWorldState(wtb.Engine, true)
	if _, err := ws.CreateObject(ctx, "repo/invalid", &bucket.ObjectRef{}); err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, ws, "repo/invalid", git_world.GitRepoTypeID); err != nil {
		t.Fatal(err)
	}

	rootCursor := NewFSCursor(le, world.NewEngineWorldState(wtb.Engine, false), 13, "space-git-invalid")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	invalidHandle, _, err := rootHandle.LookupPath(ctx, "u/13/so/space-git-invalid/-/repo/invalid/-/HEAD")
	if err == nil {
		invalidHandle.Release()
		t.Fatal("expected invalid git repo projection to fail")
	}
}

func TestFSCursorProjectsPopulatedGitRepoMetadataPaths(t *testing.T) {
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

	gitOpc := world.NewLookupOpController("test-space-projection-populated-git-repo", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp("repo/populated", nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}
	workdir := memfs.New()
	var commitHash string
	_, _, err = git_world.AccessWorldObjectRepo(ctx, ws, "repo/populated", true, nil, workdir, nil, func(repo *git.Repository) error {
		hash, err := commitReadme(repo, workdir)
		if err != nil {
			return err
		}
		commitHash = hash
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	rootCursor := NewFSCursor(le, world.NewEngineWorldState(wtb.Engine, false), 12, "space-git-populated")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	headHandle, _, err := rootHandle.LookupPath(ctx, "u/12/so/space-git-populated/-/repo/populated/-/HEAD")
	if err != nil {
		t.Fatal(err)
	}
	defer headHandle.Release()
	buf := make([]byte, 64)
	n, err := headHandle.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "ref: refs/heads/master\n" {
		t.Fatalf("got HEAD %q, want %q", got, "ref: refs/heads/master\n")
	}

	refHandle, _, err := rootHandle.LookupPath(ctx, "u/12/so/space-git-populated/-/repo/populated/-/refs/heads/master")
	if err != nil {
		t.Fatal(err)
	}
	defer refHandle.Release()
	n, err = refHandle.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != commitHash+"\n" {
		t.Fatalf("got ref %q, want %q", got, commitHash+"\n")
	}

	sourceFileHandle, _, err := rootHandle.LookupPath(ctx, "u/12/so/space-git-populated/-/repo/populated/-/README.md")
	if err == nil {
		sourceFileHandle.Release()
		t.Fatal("expected source tree file to stay out of repository metadata projection")
	}
}

func TestFSCursorProjectedGitRepoHandleReacquiresAfterCommit(t *testing.T) {
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

	gitOpc := world.NewLookupOpController("test-space-projection-git-repo-reacquire", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp("repo/reacquire", nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}

	workdir := memfs.New()
	var firstHash string
	_, _, err = git_world.AccessWorldObjectRepo(ctx, ws, "repo/reacquire", true, nil, workdir, nil, func(repo *git.Repository) error {
		hash, err := commitReadmeContent(repo, workdir, "# Demo 1\n", "initial commit")
		if err != nil {
			return err
		}
		firstHash = hash
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	rootCursor := NewFSCursor(le, world.NewEngineWorldState(wtb.Engine, false), 14, "space-git-reacquire")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	refPath := "u/14/so/space-git-reacquire/-/repo/reacquire/-/refs/heads/master"
	refHandle, _, err := rootHandle.LookupPath(ctx, refPath)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 64)
	n, err := refHandle.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		refHandle.Release()
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != firstHash+"\n" {
		refHandle.Release()
		t.Fatalf("got first ref %q, want %q", got, firstHash+"\n")
	}
	refCursor, _, err := refHandle.GetOps(ctx)
	if err != nil {
		refHandle.Release()
		t.Fatal(err)
	}
	changed := make(chan struct{}, 1)
	refCursor.AddChangeCb(func(ch *unixfs.FSCursorChange) bool {
		if ch != nil && ch.Released {
			select {
			case changed <- struct{}{}:
			default:
			}
		}
		return false
	})
	defer refHandle.Release()

	var secondHash string
	_, _, err = git_world.AccessWorldObjectRepo(ctx, ws, "repo/reacquire", true, nil, workdir, nil, func(repo *git.Repository) error {
		hash, err := commitReadmeContent(repo, workdir, "# Demo 2\n", "second commit")
		if err != nil {
			return err
		}
		secondHash = hash
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for ref cursor to change after commit %q", secondHash+"\n")
	}

	nextHandle, _, err := rootHandle.LookupPath(ctx, refPath)
	if err != nil {
		t.Fatal(err)
	}
	defer nextHandle.Release()

	n, err = nextHandle.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != secondHash+"\n" {
		t.Fatalf("got second ref %q, want %q", got, secondHash+"\n")
	}
}

func TestFSCursorGitResourcesAgreeAfterRepoWrite(t *testing.T) {
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

	unixfsOpc := world.NewLookupOpController("test-space-projection-git-agreement-unixfs", wtb.EngineID, unixfs_world.LookupFsOp)
	if _, err := wtb.Bus.AddController(ctx, unixfsOpc, nil); err != nil {
		t.Fatal(err)
	}
	gitOpc := world.NewLookupOpController("test-space-projection-git-agreement", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	repoKey := "repo/agreement"
	worktreeKey := repoKey + "/worktree"
	workdirKey := repoKey + "/workdir"
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp(repoKey, nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}

	workdir := memfs.New()
	_, _, err = git_world.AccessWorldObjectRepo(ctx, ws, repoKey, true, nil, workdir, nil, func(repo *git.Repository) error {
		_, err := commitReadmeContent(repo, workdir, "# Demo 1\n", "initial commit")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	workdirRef := &unixfs_world.UnixfsRef{
		ObjectKey: workdirKey,
		FsType:    unixfs_world.FSType_FSType_FS_NODE,
	}
	if err := git_world.CreateWorldObjectWorktree(
		ctx,
		le,
		ws,
		worktreeKey,
		repoKey,
		workdirRef,
		true,
		&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("master"), Force: true},
		sender,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}

	var secondHash string
	err = git_world.AccessWorldObjectRepoWithWorktree(ctx, le, ws, repoKey, worktreeKey, time.Now(), true, sender, func(repo *git.Repository, workdir billy.Filesystem) error {
		hash, err := commitReadmeContent(repo, workdir, "# Demo 2\n", "second commit")
		if err != nil {
			return err
		}
		secondHash = hash
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	readWS := world.NewEngineWorldState(wtb.Engine, false)
	rootCursor := NewFSCursor(le, readWS, 15, "space-git-agreement")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	spaceRef := readHandlePath(t, ctx, rootHandle, "u/15/so/space-git-agreement/-/repo/agreement/-/refs/heads/master", 64)
	if spaceRef != secondHash+"\n" {
		t.Fatalf("space projection ref %q, want %q", spaceRef, secondHash+"\n")
	}

	repoCursor, err := git_repofs.OpenRepoFSCursor(ctx, readWS, repoKey, false)
	if err != nil {
		t.Fatal(err)
	}
	repoHandle, err := unixfs.NewFSHandle(repoCursor)
	if err != nil {
		repoCursor.Release()
		t.Fatal(err)
	}
	defer repoHandle.Release()
	explicitRef := readHandlePath(t, ctx, repoHandle, "refs/heads/master", 64)
	if explicitRef != secondHash+"\n" {
		t.Fatalf("explicit repo fs ref %q, want %q", explicitRef, secondHash+"\n")
	}

	var repoSnap resource_git.RepoSnapshot
	_, _, err = git_world.AccessWorldObjectRepo(ctx, readWS, repoKey, false, nil, nil, nil, func(repo *git.Repository) error {
		return resource_git.SnapshotRepo(repo, &repoSnap)
	})
	if err != nil {
		t.Fatal(err)
	}
	repoResource := resource_git.NewGitRepoResource(readWS, repoKey, &repoSnap)
	resClient, cleanup := newTestResourceClient(t, ctx, repoResource.GetMux())
	defer cleanup()

	repoRef := resClient.AccessRootResource()
	defer repoRef.Release()
	repoClient, err := repoRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	repoSvc := s4wave_git.NewSRPCGitRepoResourceServiceClient(repoClient)
	resolve, err := repoSvc.ResolveRef(ctx, &s4wave_git.ResolveRefRequest{RefName: "master"})
	if err != nil {
		t.Fatal(err)
	}
	if resolve.GetCommitHash() != secondHash {
		t.Fatalf("repo resource resolved %q, want %q", resolve.GetCommitHash(), secondHash)
	}

	treeResp, err := repoSvc.GetTreeResource(ctx, &s4wave_git.GetTreeResourceRequest{RefName: "master"})
	if err != nil {
		t.Fatal(err)
	}
	treeRef := resClient.CreateResourceReference(treeResp.GetResourceId())
	defer treeRef.Release()
	treeClient, err := treeRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	treeSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(treeClient)
	lookup, err := treeSvc.Lookup(ctx, &s4wave_unixfs.HandleLookupRequest{Name: "README.md"})
	if err != nil {
		t.Fatal(err)
	}
	fileRef := resClient.CreateResourceReference(lookup.GetResourceId())
	defer fileRef.Release()
	fileClient, err := fileRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	fileSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(fileClient)
	fileResp, err := fileSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{Length: 32})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(fileResp.GetData()); got != "# Demo 2\n" {
		t.Fatalf("source tree README %q, want %q", got, "# Demo 2\n")
	}

	worktreeResource := resource_git.NewGitWorktreeResource(readWS, wtb.Engine, worktreeKey, &resource_git.WorktreeSnapshot{
		RepoObjectKey:    repoKey,
		WorkdirObjectKey: workdirKey,
		WorkdirRef:       workdirRef,
		CheckedOutRef:    "master",
		HeadCommitHash:   secondHash,
		HasWorkdir:       true,
	})
	status, err := watchGitWorktreeStatus(ctx, worktreeResource)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.GetEntries()) != 0 {
		t.Fatalf("expected clean worktree status, got %d entries", len(status.GetEntries()))
	}

	worktreeCursor, err := openGitWorktreeCursor(ctx, le, readWS, worktreeKey)
	if err != nil {
		t.Fatal(err)
	}
	worktreeHandle, err := unixfs.NewFSHandle(worktreeCursor)
	if err != nil {
		worktreeCursor.Release()
		t.Fatal(err)
	}
	defer worktreeHandle.Release()
	worktreeReadme := readHandlePath(t, ctx, worktreeHandle, "README.md", 32)
	if worktreeReadme != "# Demo 2\n" {
		t.Fatalf("worktree README %q, want %q", worktreeReadme, "# Demo 2\n")
	}
}

func TestFSCursorGitProjectionVerticalSlice(t *testing.T) {
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

	unixfsOpc := world.NewLookupOpController("test-space-projection-git-vertical-unixfs", wtb.EngineID, unixfs_world.LookupFsOp)
	if _, err := wtb.Bus.AddController(ctx, unixfsOpc, nil); err != nil {
		t.Fatal(err)
	}
	gitOpc := world.NewLookupOpController("test-space-projection-git-vertical", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	repoKey := "repo/vertical"
	worktreeKey := repoKey + "/worktree"
	workdirKey := repoKey + "/workdir"
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp(repoKey, nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}
	workdir := memfs.New()
	var commitHash string
	_, _, err = git_world.AccessWorldObjectRepo(ctx, ws, repoKey, true, nil, workdir, nil, func(repo *git.Repository) error {
		hash, err := commitReadmeContent(repo, workdir, "# Vertical\n", "initial commit")
		if err != nil {
			return err
		}
		commitHash = hash
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	workdirRef := &unixfs_world.UnixfsRef{
		ObjectKey: workdirKey,
		FsType:    unixfs_world.FSType_FSType_FS_NODE,
	}
	if err := git_world.CreateWorldObjectWorktree(
		ctx,
		le,
		ws,
		worktreeKey,
		repoKey,
		workdirRef,
		true,
		&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("master"), Force: true},
		sender,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}

	readWS := world.NewEngineWorldState(wtb.Engine, false)
	worktreeResource := resource_git.NewGitWorktreeResource(readWS, wtb.Engine, worktreeKey, &resource_git.WorktreeSnapshot{
		RepoObjectKey:    repoKey,
		WorkdirObjectKey: workdirKey,
		WorkdirRef:       workdirRef,
		CheckedOutRef:    "master",
		HeadCommitHash:   commitHash,
		HasWorkdir:       true,
	})
	beforeStatus, err := watchGitWorktreeStatus(ctx, worktreeResource)
	if err != nil {
		t.Fatal(err)
	}

	rootCursor := NewFSCursor(le, readWS, 16, "space-git-vertical")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	headsPath := "u/16/so/space-git-vertical/-/repo/vertical/-/refs/heads"
	headsHandle, _, err := rootHandle.LookupPath(ctx, headsPath)
	if err != nil {
		t.Fatal(err)
	}
	headsCursor, _, err := headsHandle.GetOps(ctx)
	if err != nil {
		headsHandle.Release()
		t.Fatal(err)
	}
	changed := make(chan struct{}, 1)
	headsCursor.AddChangeCb(func(ch *unixfs.FSCursorChange) bool {
		if ch != nil && ch.Released {
			select {
			case changed <- struct{}{}:
			default:
			}
		}
		return false
	})
	defer headsHandle.Release()

	writeCursor, err := git_repofs.OpenRepoFSCursor(ctx, ws, repoKey, true)
	if err != nil {
		t.Fatal(err)
	}
	writeHandle, err := unixfs.NewFSHandle(writeCursor)
	if err != nil {
		writeCursor.Release()
		t.Fatal(err)
	}
	defer writeHandle.Release()
	writeHeads, _, err := writeHandle.LookupPath(ctx, "refs/heads")
	if err != nil {
		t.Fatal(err)
	}
	branchContent := []byte(commitHash + "\n")
	if err := writeHeads.MknodWithContent(ctx, "integration", unixfs.NewFSCursorNodeType_File(), int64(len(branchContent)), bytes.NewReader(branchContent), 0o644, time.Now()); err != nil {
		writeHeads.Release()
		t.Fatal(err)
	}
	writeHeads.Release()

	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for projected repo handle invalidation")
	}

	projectedRef := readHandlePath(t, ctx, rootHandle, headsPath+"/integration", 64)
	if projectedRef != commitHash+"\n" {
		t.Fatalf("projected integration ref %q, want %q", projectedRef, commitHash+"\n")
	}
	if _, _, err := rootHandle.LookupPath(ctx, "u/16/so/space-git-vertical/-/repo/vertical/-/README.md"); err == nil {
		t.Fatal("repo projection exposed source-tree README")
	}

	var repoSnap resource_git.RepoSnapshot
	_, _, err = git_world.AccessWorldObjectRepo(ctx, readWS, repoKey, false, nil, nil, nil, func(repo *git.Repository) error {
		return resource_git.SnapshotRepo(repo, &repoSnap)
	})
	if err != nil {
		t.Fatal(err)
	}
	repoResource := resource_git.NewGitRepoResource(readWS, repoKey, &repoSnap)
	resClient, cleanup := newTestResourceClient(t, ctx, repoResource.GetMux())
	defer cleanup()
	repoRef := resClient.AccessRootResource()
	defer repoRef.Release()
	repoClient, err := repoRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	repoSvc := s4wave_git.NewSRPCGitRepoResourceServiceClient(repoClient)
	treeResp, err := repoSvc.GetTreeResource(ctx, &s4wave_git.GetTreeResourceRequest{RefName: "master"})
	if err != nil {
		t.Fatal(err)
	}
	treeRef := resClient.CreateResourceReference(treeResp.GetResourceId())
	defer treeRef.Release()
	treeClient, err := treeRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	treeSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(treeClient)
	lookup, err := treeSvc.Lookup(ctx, &s4wave_unixfs.HandleLookupRequest{Name: "README.md"})
	if err != nil {
		t.Fatal(err)
	}
	fileRef := resClient.CreateResourceReference(lookup.GetResourceId())
	defer fileRef.Release()
	fileClient, err := fileRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	fileSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(fileClient)
	fileResp, err := fileSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{Length: 32})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(fileResp.GetData()); got != "# Vertical\n" {
		t.Fatalf("source tree README %q, want %q", got, "# Vertical\n")
	}

	afterStatus, err := watchGitWorktreeStatus(ctx, worktreeResource)
	if err != nil {
		t.Fatal(err)
	}
	if !statusEntriesEqual(beforeStatus.GetEntries(), afterStatus.GetEntries()) {
		t.Fatal("worktree status changed after repo filesystem write")
	}
	worktreeCursor, err := openGitWorktreeCursor(ctx, le, readWS, worktreeKey)
	if err != nil {
		t.Fatal(err)
	}
	worktreeHandle, err := unixfs.NewFSHandle(worktreeCursor)
	if err != nil {
		worktreeCursor.Release()
		t.Fatal(err)
	}
	defer worktreeHandle.Release()
	worktreeReadme := readHandlePath(t, ctx, worktreeHandle, "README.md", 32)
	if worktreeReadme != "# Vertical\n" {
		t.Fatalf("worktree README %q, want %q", worktreeReadme, "# Vertical\n")
	}
}

func TestFSCursorProjectsGitWorktreePaths(t *testing.T) {
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

	unixfsOpc := world.NewLookupOpController("test-space-projection-git-unixfs", wtb.EngineID, unixfs_world.LookupFsOp)
	if _, err := wtb.Bus.AddController(ctx, unixfsOpc, nil); err != nil {
		t.Fatal(err)
	}
	gitOpc := world.NewLookupOpController("test-space-projection-git", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	if _, err := ws.CreateObject(ctx, "repo/demo", &bucket.ObjectRef{}); err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, ws, "repo/demo", git_world.GitRepoTypeID); err != nil {
		t.Fatal(err)
	}

	workdirRef := &unixfs_world.UnixfsRef{
		ObjectKey: "repo/demo/workdir",
		FsType:    unixfs_world.FSType_FSType_FS_NODE,
	}
	if err := git_world.CreateWorldObjectWorktree(
		ctx,
		le,
		ws,
		"repo/demo/worktree",
		"repo/demo",
		workdirRef,
		true,
		nil,
		sender,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}

	tx, err := wtb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	workdirCursor, _ := unixfs_world.NewFSCursorWithWriter(
		ctx,
		le,
		tx,
		"repo/demo/workdir",
		unixfs_world.FSType_FSType_FS_NODE,
		sender,
	)
	workdirHandle, err := unixfs.NewFSHandle(workdirCursor)
	if err != nil {
		workdirCursor.Release()
		t.Fatal(err)
	}
	defer workdirHandle.Release()

	now := time.Now()
	if err := workdirHandle.Mknod(ctx, true, []string{"README.md"}, unixfs.NewFSCursorNodeType_File(), 0o644, now); err != nil {
		t.Fatal(err)
	}
	fileHandle, _, err := workdirHandle.LookupPath(ctx, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := fileHandle.WriteAt(ctx, 0, []byte("git worktree file"), now); err != nil {
		fileHandle.Release()
		t.Fatal(err)
	}
	fileHandle.Release()
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	rootCursor := NewFSCursor(le, world.NewEngineWorldState(wtb.Engine, false), 11, "space-git")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	projectedFile, _, err := rootHandle.LookupPath(ctx, "u/11/so/space-git/-/repo/demo/worktree/-/README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer projectedFile.Release()

	buf := make([]byte, 32)
	n, err := projectedFile.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "git worktree file" {
		t.Fatalf("got %q, want %q", got, "git worktree file")
	}
}

func commitReadme(repo *git.Repository, workdir billy.Filesystem) (string, error) {
	return commitReadmeContent(repo, workdir, "# Demo\n", "initial commit")
}

func commitReadmeContent(repo *git.Repository, workdir billy.Filesystem, content, message string) (string, error) {
	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}
	f, err := workdir.Create("README.md")
	if err != nil {
		return "", err
	}
	if _, err := f.Write([]byte(content)); err != nil {
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
	hash, err := wt.Commit(message, &git.CommitOptions{
		Author:    sig,
		Committer: sig,
	})
	if err != nil {
		return "", err
	}
	return hash.String(), nil
}

func readHandlePath(t *testing.T, ctx context.Context, handle *unixfs.FSHandle, path string, size int) string {
	t.Helper()
	child, _, err := handle.LookupPath(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()
	buf := make([]byte, size)
	n, err := child.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	return string(buf[:n])
}

func statusEntriesEqual(a, b []*s4wave_git.StatusEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i, av := range a {
		bv := b[i]
		if av.GetFilePath() != bv.GetFilePath() ||
			av.GetStagingStatus() != bv.GetStagingStatus() ||
			av.GetWorktreeStatus() != bv.GetWorktreeStatus() {
			return false
		}
	}
	return true
}

func watchGitWorktreeStatus(
	ctx context.Context,
	resource *resource_git.GitWorktreeResource,
) (*s4wave_git.WatchStatusResponse, error) {
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := newTestGitWatchStatusStream(watchCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- resource.WatchStatus(&s4wave_git.WatchStatusRequest{}, stream)
	}()
	var resp *s4wave_git.WatchStatusResponse
	select {
	case resp = <-stream.msgs:
	case <-watchCtx.Done():
		return nil, watchCtx.Err()
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			return nil, err
		}
	case <-time.After(time.Second):
		return nil, context.DeadlineExceeded
	}
	return resp, nil
}

func newTestResourceClient(
	t *testing.T,
	ctx context.Context,
	rootMux srpc.Invoker,
) (*resource_client.Client, func()) {
	t.Helper()

	clientPipe, serverPipe := net.Pipe()
	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		clientPipe.Close()
		serverPipe.Close()
		t.Fatal(err)
	}
	srpcClient := srpc.NewClientWithMuxedConn(clientMp)
	resourceSrv := resource_server.NewResourceServer(rootMux)
	serverMux := srpc.NewMux()
	if err := resourceSrv.Register(serverMux); err != nil {
		clientPipe.Close()
		serverPipe.Close()
		t.Fatal(err)
	}
	server := srpc.NewServer(serverMux)
	serverMp, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		clientPipe.Close()
		serverPipe.Close()
		t.Fatal(err)
	}
	go func() {
		_ = server.AcceptMuxedConn(ctx, serverMp)
	}()

	resourceSvc := resource.NewSRPCResourceServiceClient(srpcClient)
	resClient, err := resource_client.NewClient(ctx, resourceSvc)
	if err != nil {
		clientPipe.Close()
		serverPipe.Close()
		t.Fatal(err)
	}

	return resClient, func() {
		resClient.Release()
		clientPipe.Close()
		serverPipe.Close()
	}
}
