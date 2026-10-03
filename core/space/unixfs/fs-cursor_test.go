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
	git_world "github.com/s4wave/spacewave/db/git/world"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
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

func TestFSCursorProjectsPopulatedGitRepoMetadataPaths(t *testing.T) {
	// Create the context and debug logger for the projected filesystem test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for the projected World.
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	// Start the World engine and retain it for the test lifetime.
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Register Git operations on the World engine.
	gitOpc := world.NewLookupOpController("test-space-projection-populated-git-repo", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	// Initialize the Git repository in the writable World.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp("repo/populated", nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}

	// Commit the initial README into the World repository.
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

	// Open the projected filesystem root and retain its handle.
	rootCursor := NewFSCursor(le, world.NewEngineWorldState(wtb.Engine, false), 12, "space-git-populated")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	// Open the projected repository HEAD.
	headHandle, _, err := rootHandle.LookupPath(ctx, "u/12/so/space-git-populated/-/repo/populated/-/HEAD")
	if err != nil {
		t.Fatal(err)
	}
	defer headHandle.Release()

	// Read the projected HEAD contents.
	buf := make([]byte, 64)
	n, err := headHandle.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}

	// Verify that the projected HEAD points to the master branch.
	if got := string(buf[:n]); got != "ref: refs/heads/master\n" {
		t.Fatalf("got HEAD %q, want %q", got, "ref: refs/heads/master\n")
	}

	// Open the projected master branch reference.
	refHandle, _, err := rootHandle.LookupPath(ctx, "u/12/so/space-git-populated/-/repo/populated/-/refs/heads/master")
	if err != nil {
		t.Fatal(err)
	}
	defer refHandle.Release()

	// Read the projected master branch contents.
	n, err = refHandle.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}

	// Verify that the projected branch identifies the committed README.
	if got := string(buf[:n]); got != commitHash+"\n" {
		t.Fatalf("got ref %q, want %q", got, commitHash+"\n")
	}

	// Verify that repository metadata excludes source tree files.
	sourceFileHandle, _, err := rootHandle.LookupPath(ctx, "u/12/so/space-git-populated/-/repo/populated/-/README.md")
	if err == nil {
		sourceFileHandle.Release()
		t.Fatal("expected source tree file to stay out of repository metadata projection")
	}
}

func TestFSCursorProjectedGitRepoHandleReacquiresAfterCommit(t *testing.T) {
	// Create the context and debug logger for the projected filesystem test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for the projected World.
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	// Start the World engine and retain it for the test lifetime.
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Register Git operations on the World engine.
	gitOpc := world.NewLookupOpController("test-space-projection-git-repo-reacquire", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	// Initialize the Git repository in the writable World.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp("repo/reacquire", nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}

	// Commit the initial README into the World repository.
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

	// Open the projected filesystem root and retain its handle.
	rootCursor := NewFSCursor(le, world.NewEngineWorldState(wtb.Engine, false), 14, "space-git-reacquire")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	// Open the projected branch handle before the next commit.
	refPath := "u/14/so/space-git-reacquire/-/repo/reacquire/-/refs/heads/master"
	refHandle, _, err := rootHandle.LookupPath(ctx, refPath)
	if err != nil {
		t.Fatal(err)
	}

	// Read the branch contents before committing another README.
	buf := make([]byte, 64)
	n, err := refHandle.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		refHandle.Release()
		t.Fatal(err)
	}

	// Verify that the branch initially identifies the first commit.
	if got := string(buf[:n]); got != firstHash+"\n" {
		refHandle.Release()
		t.Fatalf("got first ref %q, want %q", got, firstHash+"\n")
	}

	// Watch the branch cursor for invalidation after a repository commit.
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

	// Commit a second README into the same World repository.
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

	// Wait for the branch cursor to report the repository change.
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for ref cursor to change after commit %q", secondHash+"\n")
	}

	// Reacquire the projected branch handle after invalidation.
	nextHandle, _, err := rootHandle.LookupPath(ctx, refPath)
	if err != nil {
		t.Fatal(err)
	}
	defer nextHandle.Release()

	// Read the branch contents through the reacquired handle.
	n, err = nextHandle.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}

	// Verify that the reacquired branch identifies the second commit.
	if got := string(buf[:n]); got != secondHash+"\n" {
		t.Fatalf("got second ref %q, want %q", got, secondHash+"\n")
	}
}

func TestFSCursorGitResourcesAgreeAfterRepoWrite(t *testing.T) {
	// Create the context and debug logger for the projected filesystem test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for the projected World.
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	// Start the World engine and retain it for the test lifetime.
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Register UnixFS operations for the Git worktree directory.
	unixfsOpc := world.NewLookupOpController("test-space-projection-git-agreement-unixfs", wtb.EngineID, unixfs_world.LookupFsOp)
	if _, err := wtb.Bus.AddController(ctx, unixfsOpc, nil); err != nil {
		t.Fatal(err)
	}

	// Register Git operations on the World engine.
	gitOpc := world.NewLookupOpController("test-space-projection-git-agreement", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	// Initialize the Git repository in the writable World.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	repoKey := "repo/agreement"
	worktreeKey := repoKey + "/worktree"
	workdirKey := repoKey + "/workdir"
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp(repoKey, nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}

	// Commit the initial README into the World repository.
	workdir := memfs.New()
	_, _, err = git_world.AccessWorldObjectRepo(ctx, ws, repoKey, true, nil, workdir, nil, func(repo *git.Repository) error {
		_, err := commitReadmeContent(repo, workdir, "# Demo 1\n", "initial commit")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create a World worktree checked out at the master branch.
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

	// Commit a second README through the World worktree.
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

	// Open the projected filesystem root and retain its handle.
	readWS := world.NewEngineWorldState(wtb.Engine, false)
	rootCursor := NewFSCursor(le, readWS, 15, "space-git-agreement")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	// Verify that the space projection exposes the second commit.
	spaceRef := readHandlePath(t, ctx, rootHandle, "u/15/so/space-git-agreement/-/repo/agreement/-/refs/heads/master", 64)
	if spaceRef != secondHash+"\n" {
		t.Fatalf("space projection ref %q, want %q", spaceRef, secondHash+"\n")
	}

	// Open the repository filesystem directly for comparison.
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

	// Verify that the repository filesystem exposes the same commit.
	explicitRef := readHandlePath(t, ctx, repoHandle, "refs/heads/master", 64)
	if explicitRef != secondHash+"\n" {
		t.Fatalf("explicit repo fs ref %q, want %q", explicitRef, secondHash+"\n")
	}

	// Snapshot the repository and connect its resource service.
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

	// Access the repository resource through its client reference.
	repoRef := resClient.AccessRootResource()
	defer repoRef.Release()
	repoClient, err := repoRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}

	// Resolve the master branch through the repository resource service.
	repoSvc := s4wave_git.NewSRPCGitRepoResourceServiceClient(repoClient)
	resolve, err := repoSvc.ResolveRef(ctx, &s4wave_git.ResolveRefRequest{RefName: "master"})
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the repository resource resolves the second commit.
	if resolve.GetCommitHash() != secondHash {
		t.Fatalf("repo resource resolved %q, want %q", resolve.GetCommitHash(), secondHash)
	}

	// Request the committed master tree through the repository resource service.
	treeResp, err := repoSvc.GetTreeResource(ctx, &s4wave_git.GetTreeResourceRequest{RefName: "master"})
	if err != nil {
		t.Fatal(err)
	}

	// Access the returned tree resource through its client reference.
	treeRef := resClient.CreateResourceReference(treeResp.GetResourceId())
	defer treeRef.Release()
	treeClient, err := treeRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}

	// Look up README.md in the committed tree resource.
	treeSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(treeClient)
	lookup, err := treeSvc.Lookup(ctx, &s4wave_unixfs.HandleLookupRequest{Name: "README.md"})
	if err != nil {
		t.Fatal(err)
	}

	// Access the returned README resource through its client reference.
	fileRef := resClient.CreateResourceReference(lookup.GetResourceId())
	defer fileRef.Release()
	fileClient, err := fileRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}

	// Read README.md through the filesystem resource service.
	fileSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(fileClient)
	fileResp, err := fileSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{Length: 32})
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the tree resource returns the committed README contents.
	if got := string(fileResp.GetData()); got != "# Demo 2\n" {
		t.Fatalf("source tree README %q, want %q", got, "# Demo 2\n")
	}

	// Read the worktree status through its World resource.
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

	// Verify that the committed worktree has no pending changes.
	if len(status.GetEntries()) != 0 {
		t.Fatalf("expected clean worktree status, got %d entries", len(status.GetEntries()))
	}

	// Open the worktree filesystem from its stored directory reference.
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

	// Verify that the worktree filesystem returns the committed README.
	worktreeReadme := readHandlePath(t, ctx, worktreeHandle, "README.md", 32)
	if worktreeReadme != "# Demo 2\n" {
		t.Fatalf("worktree README %q, want %q", worktreeReadme, "# Demo 2\n")
	}
}

func TestFSCursorGitProjectionVerticalSlice(t *testing.T) {
	// Create the context and debug logger for the projected filesystem test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for the projected World.
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	// Start the World engine and retain it for the test lifetime.
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Register UnixFS operations for the Git worktree directory.
	unixfsOpc := world.NewLookupOpController("test-space-projection-git-vertical-unixfs", wtb.EngineID, unixfs_world.LookupFsOp)
	if _, err := wtb.Bus.AddController(ctx, unixfsOpc, nil); err != nil {
		t.Fatal(err)
	}

	// Register Git operations on the World engine.
	gitOpc := world.NewLookupOpController("test-space-projection-git-vertical", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	// Initialize the Git repository in the writable World.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	repoKey := "repo/vertical"
	worktreeKey := repoKey + "/worktree"
	workdirKey := repoKey + "/workdir"
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp(repoKey, nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}

	// Commit the initial README into the World repository.
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

	// Create a World worktree checked out at the master branch.
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

	// Capture the worktree status before changing repository metadata.
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

	// Open the projected filesystem root and retain its handle.
	rootCursor := NewFSCursor(le, readWS, 16, "space-git-vertical")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	// Watch the projected branch directory for cursor invalidation.
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

	// Open a writable handle on the repository metadata filesystem.
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

	// Create the integration branch through the repository filesystem.
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

	// Wait for the projected branch directory to report invalidation.
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for projected repo handle invalidation")
	}

	// Verify that the space projection exposes the new integration branch.
	projectedRef := readHandlePath(t, ctx, rootHandle, headsPath+"/integration", 64)
	if projectedRef != commitHash+"\n" {
		t.Fatalf("projected integration ref %q, want %q", projectedRef, commitHash+"\n")
	}

	// Verify that repository projection still excludes source tree files.
	if _, _, err := rootHandle.LookupPath(ctx, "u/16/so/space-git-vertical/-/repo/vertical/-/README.md"); err == nil {
		t.Fatal("repo projection exposed source-tree README")
	}

	// Snapshot the repository and connect its resource service.
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

	// Access the repository resource through its client reference.
	repoRef := resClient.AccessRootResource()
	defer repoRef.Release()
	repoClient, err := repoRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}

	// Request the committed master tree through the repository resource service.
	repoSvc := s4wave_git.NewSRPCGitRepoResourceServiceClient(repoClient)
	treeResp, err := repoSvc.GetTreeResource(ctx, &s4wave_git.GetTreeResourceRequest{RefName: "master"})
	if err != nil {
		t.Fatal(err)
	}

	// Access the returned tree resource through its client reference.
	treeRef := resClient.CreateResourceReference(treeResp.GetResourceId())
	defer treeRef.Release()
	treeClient, err := treeRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}

	// Look up README.md in the committed tree resource.
	treeSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(treeClient)
	lookup, err := treeSvc.Lookup(ctx, &s4wave_unixfs.HandleLookupRequest{Name: "README.md"})
	if err != nil {
		t.Fatal(err)
	}

	// Access the returned README resource through its client reference.
	fileRef := resClient.CreateResourceReference(lookup.GetResourceId())
	defer fileRef.Release()
	fileClient, err := fileRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}

	// Read README.md through the filesystem resource service.
	fileSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(fileClient)
	fileResp, err := fileSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{Length: 32})
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the tree resource returns the committed README contents.
	if got := string(fileResp.GetData()); got != "# Vertical\n" {
		t.Fatalf("source tree README %q, want %q", got, "# Vertical\n")
	}

	// Read the worktree status after the repository metadata write.
	afterStatus, err := watchGitWorktreeStatus(ctx, worktreeResource)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the metadata write leaves worktree status unchanged.
	if !statusEntriesEqual(beforeStatus.GetEntries(), afterStatus.GetEntries()) {
		t.Fatal("worktree status changed after repo filesystem write")
	}

	// Open the worktree filesystem from its stored directory reference.
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

	// Verify that the worktree filesystem returns the committed README.
	worktreeReadme := readHandlePath(t, ctx, worktreeHandle, "README.md", 32)
	if worktreeReadme != "# Vertical\n" {
		t.Fatalf("worktree README %q, want %q", worktreeReadme, "# Vertical\n")
	}
}

func commitReadme(repo *git.Repository, workdir billy.Filesystem) (string, error) {
	return commitReadmeContent(repo, workdir, "# Demo\n", "initial commit")
}

func commitReadmeContent(repo *git.Repository, workdir billy.Filesystem, content, message string) (string, error) {
	// Open the Git worktree used to commit README.md.
	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}

	// Write and close README.md in the worktree filesystem.
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

	// Stage README.md for the repository commit.
	if _, err := wt.Add("README.md"); err != nil {
		return "", err
	}

	// Commit the staged README with a test author and committer.
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
	// Open the requested child handle for the test read.
	t.Helper()
	child, _, err := handle.LookupPath(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	// Read the child contents through its filesystem handle.
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
	// Start a cancellable watch on the Git worktree status resource.
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := newTestGitWatchStatusStream(watchCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- resource.WatchStatus(&s4wave_git.WatchStatusRequest{}, stream)
	}()

	// Receive the first worktree status snapshot.
	var resp *s4wave_git.WatchStatusResponse
	select {
	case resp = <-stream.msgs:
	case <-watchCtx.Done():
		return nil, watchCtx.Err()
	}

	// Cancel the status watch and wait for the stream to finish.
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
	// Attribute resource client setup failures to the calling test.
	t.Helper()

	// Connect the resource client to an in-memory multiplexed transport.
	clientPipe, serverPipe := net.Pipe()
	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		clientPipe.Close()
		serverPipe.Close()
		t.Fatal(err)
	}
	srpcClient := srpc.NewClientWithMuxedConn(clientMp)

	// Register the root resource service on the server multiplexer.
	resourceSrv := resource_server.NewResourceServer(rootMux)
	serverMux := srpc.NewMux()
	if err := resourceSrv.Register(serverMux); err != nil {
		clientPipe.Close()
		serverPipe.Close()
		t.Fatal(err)
	}

	// Accept the server side of the resource transport.
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

	// Construct the resource client over the connected service.
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
