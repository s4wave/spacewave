package forge_lib_git_lazyrepo

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
	forge_lib_git_allocation "github.com/s4wave/spacewave/forge/lib/git/allocation"
)

func TestMountedRepoResolverResolvesNearestRepoRoot(t *testing.T) {
	// Configure parent and submodule mounts with distinct repository identities.
	resolver, err := NewMountedRepoResolver([]RepoMount{
		{
			MountName:      "workspace",
			MountPath:      "/workspace",
			RepoRootPath:   "repos/spacewave",
			RepoObjectKey:  "repo/spacewave",
			BaseCommitHash: "1111111111111111111111111111111111111111",
		},
		{
			MountName:      "workspace",
			MountPath:      "/workspace",
			RepoRootPath:   "repos/spacewave/vendor/submodule",
			RepoObjectKey:  "repo/submodule",
			BaseCommitHash: "2222222222222222222222222222222222222222",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Resolve a guest file beneath the submodule mount.
	resolved, err := resolver.ResolveGuestPath("/workspace/repos/spacewave/vendor/submodule/pkg/file.go")
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the nearest repository supplies the file provenance.
	if resolved.RepoObjectKey != "repo/submodule" ||
		resolved.RepoRootPath != "repos/spacewave/vendor/submodule" ||
		resolved.GuestPath != "/workspace/repos/spacewave/vendor/submodule/pkg/file.go" ||
		!slices.Equal(resolved.RepoRelativePath, []string{"pkg", "file.go"}) ||
		resolved.Mount.MountName != "workspace" ||
		resolved.Mount.MountPath != "/workspace" {
		t.Fatalf("unexpected resolution: %+v", resolved)
	}
}

func TestMountedRepoResolverKeepsRootPathFamily(t *testing.T) {
	// Configure a repository mounted at the filesystem root.
	resolver, err := NewMountedRepoResolver([]RepoMount{{
		MountName:      "repo",
		MountPath:      "/repo",
		RepoRootPath:   ".",
		RepoObjectKey:  "repo/root",
		BaseCommitHash: "1111111111111111111111111111111111111111",
	}})
	if err != nil {
		t.Fatal(err)
	}

	// Resolve a file beneath the root repository mount.
	resolved, err := resolver.ResolveGuestPath("/repo/README.md")
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the root path family remains explicit.
	if resolved.PathFamily == "" || resolved.PathFamily != "." ||
		resolved.RepoRootPath != "." ||
		!slices.Equal(resolved.RepoRelativePath, []string{"README.md"}) {
		t.Fatalf("unexpected root-mounted resolution: %+v", resolved)
	}
}

func TestLazyRepoCursorAllocatesOnceAndSwitchesToWritableTree(t *testing.T) {
	// Prepare separate read and writable repository trees.
	ctx := context.Background()
	readRoot := newMemDir("", map[string]*memNode{
		"repos": newMemDir("repos", map[string]*memNode{
			"spacewave": newMemDir("spacewave", map[string]*memNode{
				"README.md": newMemFile("README.md", []byte("read-mode")),
			}),
		}),
	})
	writableRoot := newMemDir("", map[string]*memNode{
		"repos": newMemDir("repos", map[string]*memNode{
			"spacewave": newMemDir("spacewave", map[string]*memNode{
				"README.md": newMemFile("README.md", []byte("read-mode")),
			}),
		}),
	})

	// Configure the allocator to open the writable repository tree.
	allocator := &fakeAllocator{
		open: func(ctx context.Context) (unixfs.FSCursor, error) {
			return newMemCursor(writableRoot.children["repos"].children["spacewave"]), nil
		},
	}

	// Resolve the repository mount for lazy allocation.
	resolver, err := NewMountedRepoResolver([]RepoMount{{
		MountName:      "workspace",
		MountPath:      "/workspace",
		RepoRootPath:   "repos/spacewave",
		RepoObjectKey:  "repo/spacewave",
		BaseCommitHash: "1111111111111111111111111111111111111111",
	}})
	if err != nil {
		t.Fatal(err)
	}

	// Open a lazy cursor handle over the canonical read tree.
	cursor, err := NewFSCursor(newMemCursor(readRoot), resolver, allocator)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	// Open the read file through the lazy repository handle.
	fileHandle, _, err := handle.LookupPathPts(ctx, []string{"repos", "spacewave", "README.md"})
	if err != nil {
		t.Fatal(err)
	}
	defer fileHandle.Release()

	// Capture the read cursor before the first mutation.
	oldCursor, _, err := fileHandle.GetOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the original read cursor remains live.
	if oldCursor.CheckReleased() {
		t.Fatal("expected read-mode child cursor before mutation")
	}

	// Write through the lazy file to trigger repository allocation.
	if err := fileHandle.WriteAt(ctx, 0, []byte("writable"), time.Now()); err != nil {
		t.Fatal(err)
	}

	// Verify that the first write allocates once and releases stale cursors.
	if got := allocator.allocations.Load(); got != 1 {
		t.Fatalf("expected one allocation after first write, got %d", got)
	}
	if !oldCursor.CheckReleased() {
		t.Fatal("expected stale read-mode child cursor to be released")
	}

	// Append through the allocated writable file.
	if err := fileHandle.WriteAt(ctx, 8, []byte("-tree"), time.Now()); err != nil {
		t.Fatal(err)
	}

	// Verify that the later write reuses the repository allocation.
	if got := allocator.allocations.Load(); got != 1 {
		t.Fatalf("expected allocation reuse on later write, got %d", got)
	}

	// Read the complete file through the writable tree.
	buf := make([]byte, len("writable-tree"))
	n, err := fileHandle.ReadAt(ctx, 0, buf)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the read includes both writes.
	if string(buf[:n]) != "writable-tree" {
		t.Fatalf("expected read through writable tree, got %q", string(buf[:n]))
	}

	// Verify that the canonical read file retains its original content.
	readFile := readRoot.children["repos"].children["spacewave"].children["README.md"]
	if string(readFile.data) != "read-mode" {
		t.Fatalf("canonical read tree mutated: %q", string(readFile.data))
	}

	// Verify that the allocation request carries the mutation provenance.
	if len(allocator.requests) != 1 {
		t.Fatalf("expected one allocation request, got %d", len(allocator.requests))
	}
	req := allocator.request(0)
	if req.Operation != "write-at" ||
		req.RepoObjectKey != "repo/spacewave" ||
		req.BaseCommitHash != "1111111111111111111111111111111111111111" ||
		req.PathFamily != "repos/spacewave" ||
		req.GuestPath != "/workspace/repos/spacewave/README.md" {
		t.Fatalf("unexpected allocation request: %+v", req)
	}
}

func TestLazyRepoCursorSerializesConcurrentFirstWrites(t *testing.T) {
	// Prepare separate read and writable repository trees.
	ctx := context.Background()
	readRoot := newMemDir("", map[string]*memNode{
		"repos": newMemDir("repos", map[string]*memNode{
			"spacewave": newMemDir("spacewave", map[string]*memNode{
				"README.md": newMemFile("README.md", []byte("read-mode")),
			}),
		}),
	})
	writableRoot := newMemDir("", map[string]*memNode{
		"repos": newMemDir("repos", map[string]*memNode{
			"spacewave": newMemDir("spacewave", map[string]*memNode{
				"README.md": newMemFile("README.md", []byte("read-mode")),
			}),
		}),
	})

	// Gate repository allocation while concurrent writes arrive.
	releaseAllocation := make(chan struct{})
	allocatorStarted := make(chan struct{})
	allocator := &fakeAllocator{
		onAllocate: func() {
			close(allocatorStarted)
			<-releaseAllocation
		},
		open: func(ctx context.Context) (unixfs.FSCursor, error) {
			return newMemCursor(writableRoot.children["repos"].children["spacewave"]), nil
		},
	}

	// Resolve the repository mount for lazy allocation.
	resolver, err := NewMountedRepoResolver([]RepoMount{{
		MountName:      "workspace",
		MountPath:      "/workspace",
		RepoRootPath:   "repos/spacewave",
		RepoObjectKey:  "repo/spacewave",
		BaseCommitHash: "1111111111111111111111111111111111111111",
	}})
	if err != nil {
		t.Fatal(err)
	}

	// Open a lazy cursor handle over the canonical read tree.
	cursor, err := NewFSCursor(newMemCursor(readRoot), resolver, allocator)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	// Open the read file for concurrent first writes.
	fileHandle, _, err := handle.LookupPathPts(ctx, []string{"repos", "spacewave", "README.md"})
	if err != nil {
		t.Fatal(err)
	}
	defer fileHandle.Release()

	// Start the first write and wait for repository allocation to begin.
	errCh := make(chan error, 2)
	go func() {
		errCh <- fileHandle.WriteAt(ctx, 0, []byte("first"), time.Now())
	}()
	<-allocatorStarted

	// Start the second write while repository allocation is blocked.
	go func() {
		errCh <- fileHandle.WriteAt(ctx, 0, []byte("second"), time.Now())
	}()

	// Release repository allocation and collect both write results.
	close(releaseAllocation)
	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}

	// Verify that concurrent writes share one repository allocation.
	if got := allocator.allocations.Load(); got != 1 {
		t.Fatalf("expected one serialized allocation, got %d", got)
	}
}

func TestLazyRepoCursorRenameAllocatesThenRejectsNonAtomicMove(t *testing.T) {
	// Prepare separate read and writable repository trees.
	ctx := context.Background()
	readRoot := newMemDir("", map[string]*memNode{
		"repos": newMemDir("repos", map[string]*memNode{
			"spacewave": newMemDir("spacewave", map[string]*memNode{
				"README.md": newMemFile("README.md", []byte("read-mode")),
			}),
		}),
	})
	writableRoot := newMemDir("", map[string]*memNode{
		"repos": newMemDir("repos", map[string]*memNode{
			"spacewave": newMemDir("spacewave", map[string]*memNode{
				"README.md": newMemFile("README.md", []byte("read-mode")),
			}),
		}),
	})

	// Configure the allocator to open the writable repository tree.
	allocator := &fakeAllocator{
		open: func(ctx context.Context) (unixfs.FSCursor, error) {
			return newMemCursor(writableRoot.children["repos"].children["spacewave"]), nil
		},
	}

	// Resolve the repository mount for lazy allocation.
	resolver, err := NewMountedRepoResolver([]RepoMount{{
		MountName:      "workspace",
		MountPath:      "/workspace",
		RepoRootPath:   "repos/spacewave",
		RepoObjectKey:  "repo/spacewave",
		BaseCommitHash: "1111111111111111111111111111111111111111",
	}})
	if err != nil {
		t.Fatal(err)
	}

	// Open a lazy cursor handle over the canonical read tree.
	cursor, err := NewFSCursor(newMemCursor(readRoot), resolver, allocator)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	// Open the source file and destination directory for rename.
	srcHandle, _, err := handle.LookupPathPts(ctx, []string{"repos", "spacewave", "README.md"})
	if err != nil {
		t.Fatal(err)
	}
	destParent, _, err := handle.LookupPathPts(ctx, []string{"repos", "spacewave"})
	if err != nil {
		t.Fatal(err)
	}

	// Attempt a rename through the lazy repository cursors.
	err = srcHandle.Rename(ctx, destParent, "RENAMED.md", time.Now())

	// Verify that rename requests the cross-filesystem fallback.
	if err != unixfs_errors.ErrCrossFsRename {
		t.Fatalf("expected non-atomic rename to fall back as cross-fs, got %v", err)
	}

	// Release the rename source and destination handles.
	srcHandle.Release()
	destParent.Release()

	// Verify allocation without changing either repository tree.
	if got := allocator.allocations.Load(); got != 1 {
		t.Fatalf("expected one allocation for rename, got %d", got)
	}
	if string(readRoot.children["repos"].children["spacewave"].children["README.md"].data) != "read-mode" {
		t.Fatal("rename mutated canonical read tree")
	}
	if writableRoot.children["repos"].children["spacewave"].children["RENAMED.md"] != nil {
		t.Fatal("non-atomic rename created target in writable tree")
	}
}

func TestLazyRepoCursorFirstMknodCompletesAfterAllocation(t *testing.T) {
	// Open separate repository trees through the lazy test handle.
	ctx := context.Background()
	readRoot, writableRoot, handle, allocator := newLazyRepoTestHandle(t)
	defer handle.Release()

	// Open the repository directory for file creation.
	parent, _, err := handle.LookupPathPts(ctx, []string{"repos", "spacewave"})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Release()

	// Create a file to trigger writable repository allocation.
	if err := parent.Mknod(ctx, true, []string{"created.txt"}, unixfs.NewFSCursorNodeType_File(), 0o644, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Verify that creation allocates once and changes only the writable tree.
	if got := allocator.allocations.Load(); got != 1 {
		t.Fatalf("expected one allocation for first mknod, got %d", got)
	}
	if writableRoot.children["repos"].children["spacewave"].children["created.txt"] == nil {
		t.Fatal("created file missing from writable tree")
	}
	if readRoot.children["repos"].children["spacewave"].children["created.txt"] != nil {
		t.Fatal("mknod mutated canonical read tree")
	}
}

func TestLazyRepoCursorRejectsTraversalChildNamesBeforeAllocation(t *testing.T) {
	// Open the lazy repository test handle.
	ctx := context.Background()
	_, _, handle, allocator := newLazyRepoTestHandle(t)
	defer handle.Release()

	// Open the repository directory for the traversal attempt.
	parent, _, err := handle.LookupPathPts(ctx, []string{"repos", "spacewave"})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Release()

	// Attempt file creation with a name escaping the repository directory.
	err = parent.Mknod(ctx, true, []string{"../escape.txt"}, unixfs.NewFSCursorNodeType_File(), 0o644, time.Now())

	// Verify that traversal fails before repository allocation.
	if !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("expected invalid path error, got %T %v", err, err)
	}
	if got := allocator.allocations.Load(); got != 0 {
		t.Fatalf("expected no allocation for invalid child name, got %d", got)
	}
}

func TestLazyRepoCursorAllocationWaitHonorsContextCancel(t *testing.T) {
	// Prepare separate read and writable repository trees.
	ctx := context.Background()
	readRoot := newMemDir("", map[string]*memNode{
		"repos": newMemDir("repos", map[string]*memNode{
			"spacewave": newMemDir("spacewave", map[string]*memNode{
				"README.md": newMemFile("README.md", []byte("read-mode")),
			}),
		}),
	})
	writableRoot := newMemDir("", map[string]*memNode{
		"repos": newMemDir("repos", map[string]*memNode{
			"spacewave": newMemDir("spacewave", map[string]*memNode{
				"README.md": newMemFile("README.md", []byte("read-mode")),
			}),
		}),
	})

	// Gate repository allocation while a canceled write arrives.
	releaseAllocation := make(chan struct{})
	allocatorStarted := make(chan struct{})
	allocator := &fakeAllocator{
		onAllocate: func() {
			close(allocatorStarted)
			<-releaseAllocation
		},
		open: func(ctx context.Context) (unixfs.FSCursor, error) {
			return newMemCursor(writableRoot.children["repos"].children["spacewave"]), nil
		},
	}

	// Resolve the repository mount for lazy allocation.
	resolver, err := NewMountedRepoResolver([]RepoMount{{
		MountName:      "workspace",
		MountPath:      "/workspace",
		RepoRootPath:   "repos/spacewave",
		RepoObjectKey:  "repo/spacewave",
		BaseCommitHash: "1111111111111111111111111111111111111111",
	}})
	if err != nil {
		t.Fatal(err)
	}

	// Open a lazy cursor handle over the canonical read tree.
	cursor, err := NewFSCursor(newMemCursor(readRoot), resolver, allocator)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	// Open the read file for the allocation cancellation test.
	fileHandle, _, err := handle.LookupPathPts(ctx, []string{"repos", "spacewave", "README.md"})
	if err != nil {
		t.Fatal(err)
	}
	defer fileHandle.Release()

	// Start the first write and wait for repository allocation to begin.
	errCh := make(chan error, 1)
	go func() {
		errCh <- fileHandle.WriteAt(ctx, 0, []byte("first"), time.Now())
	}()
	<-allocatorStarted

	// Cancel the second write before it waits on the allocation.
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	// Verify that the allocation wait returns context cancellation.
	if err := fileHandle.WriteAt(canceledCtx, 0, []byte("second"), time.Now()); err != context.Canceled {
		t.Fatalf("expected context.Canceled while waiting for allocation, got %v", err)
	}

	// Release repository allocation and collect the first write result.
	close(releaseAllocation)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	// Verify that the canceled write did not start a second allocation.
	if got := allocator.allocations.Load(); got != 1 {
		t.Fatalf("expected one allocation, got %d", got)
	}
}

func TestLazyRepoCursorCopyStreamsFileContent(t *testing.T) {
	// Prepare a large source payload for the streamed copy.
	ctx := context.Background()
	large := make([]byte, 1<<20)
	for idx := range large {
		large[idx] = byte(idx)
	}

	// Prepare separate read and writable trees containing the source file.
	readRoot := newMemDir("", map[string]*memNode{
		"repos": newMemDir("repos", map[string]*memNode{
			"spacewave": newMemDir("spacewave", map[string]*memNode{
				"large.bin": newMemFile("large.bin", large),
			}),
		}),
	})
	writableRoot := newMemDir("", map[string]*memNode{
		"repos": newMemDir("repos", map[string]*memNode{
			"spacewave": newMemDir("spacewave", map[string]*memNode{
				"large.bin": newMemFile("large.bin", large),
			}),
		}),
	})

	// Configure the allocator to open the writable repository tree.
	allocator := &fakeAllocator{
		open: func(ctx context.Context) (unixfs.FSCursor, error) {
			return newMemCursor(writableRoot.children["repos"].children["spacewave"]), nil
		},
	}

	// Resolve the repository mount for lazy allocation.
	resolver, err := NewMountedRepoResolver([]RepoMount{{
		MountName:      "workspace",
		MountPath:      "/workspace",
		RepoRootPath:   "repos/spacewave",
		RepoObjectKey:  "repo/spacewave",
		BaseCommitHash: "1111111111111111111111111111111111111111",
	}})
	if err != nil {
		t.Fatal(err)
	}

	// Open a lazy cursor handle over the canonical read tree.
	cursor, err := NewFSCursor(newMemCursor(readRoot), resolver, allocator)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	// Open the source file and destination directory for copying.
	srcHandle, _, err := handle.LookupPathPts(ctx, []string{"repos", "spacewave", "large.bin"})
	if err != nil {
		t.Fatal(err)
	}
	destParent, _, err := handle.LookupPathPts(ctx, []string{"repos", "spacewave"})
	if err != nil {
		t.Fatal(err)
	}

	// Copy the source file through the lazy repository cursors.
	if err := srcHandle.Copy(ctx, destParent, "large-copy.bin", time.Now()); err != nil {
		t.Fatal(err)
	}

	// Release the copy source and destination handles.
	srcHandle.Release()
	destParent.Release()

	// Verify that source reads stay smaller than the complete file.
	srcNode := writableRoot.children["repos"].children["spacewave"].children["large.bin"]
	if srcNode.maxReadLen.Load() >= int64(len(large)) {
		t.Fatalf("expected streamed copy reads below full file size, max read %d", srcNode.maxReadLen.Load())
	}
}

func TestLazyRepoCursorBlocksUnresolvedMutationBeforeAllocation(t *testing.T) {
	// Prepare a read file outside the configured repository mount.
	ctx := context.Background()
	readRoot := newMemDir("", map[string]*memNode{
		"README.md": newMemFile("README.md", []byte("read-mode")),
	})

	// Configure the allocator to expose the read tree if called.
	allocator := &fakeAllocator{
		open: func(ctx context.Context) (unixfs.FSCursor, error) {
			return newMemCursor(readRoot), nil
		},
	}

	// Resolve the repository mount for lazy allocation.
	resolver, err := NewMountedRepoResolver([]RepoMount{{
		MountName:      "workspace",
		MountPath:      "/workspace",
		RepoRootPath:   "repos/spacewave",
		RepoObjectKey:  "repo/spacewave",
		BaseCommitHash: "1111111111111111111111111111111111111111",
	}})
	if err != nil {
		t.Fatal(err)
	}

	// Open a lazy cursor handle over the canonical read tree.
	cursor, err := NewFSCursor(newMemCursor(readRoot), resolver, allocator)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	// Open the file outside the configured repository mount.
	fileHandle, _, err := handle.LookupPathPts(ctx, []string{"README.md"})
	if err != nil {
		t.Fatal(err)
	}
	defer fileHandle.Release()

	// Attempt a write without repository provenance.
	err = fileHandle.WriteAt(ctx, 0, []byte("must-not-write"), time.Now())

	// Verify the provenance failure and preserved read tree.
	var perr *ProvenanceError
	if !errors.As(err, &perr) {
		t.Fatalf("expected provenance error, got %T %v", err, err)
	}
	if perr.Operation != "write-at" || perr.MutationPath != "README.md" {
		t.Fatalf("unexpected provenance error: %+v", perr)
	}
	if got := allocator.allocations.Load(); got != 0 {
		t.Fatalf("expected no allocation for unresolved path, got %d", got)
	}
	if string(readRoot.children["README.md"].data) != "read-mode" {
		t.Fatalf("unresolved write mutated read tree: %q", string(readRoot.children["README.md"].data))
	}
}

func TestLazyRepoCursorServesV86fsMountedWritesFromWritableTree(t *testing.T) {
	// Prepare separate read and writable repository trees.
	ctx := context.Background()
	readRoot := newMemDir("", map[string]*memNode{
		"repos": newMemDir("repos", map[string]*memNode{
			"spacewave": newMemDir("spacewave", map[string]*memNode{
				"README.md": newMemFile("README.md", []byte("read-mode")),
			}),
		}),
	})
	writableRoot := newMemDir("", map[string]*memNode{
		"repos": newMemDir("repos", map[string]*memNode{
			"spacewave": newMemDir("spacewave", map[string]*memNode{
				"README.md": newMemFile("README.md", []byte("read-mode")),
			}),
		}),
	})

	// Configure the allocator to open the writable repository tree.
	allocator := &fakeAllocator{
		open: func(ctx context.Context) (unixfs.FSCursor, error) {
			return newMemCursor(writableRoot.children["repos"].children["spacewave"]), nil
		},
	}

	// Resolve the repository mount for lazy allocation.
	resolver, err := NewMountedRepoResolver([]RepoMount{{
		MountName:      "workspace",
		MountPath:      "/workspace",
		RepoRootPath:   "repos/spacewave",
		RepoObjectKey:  "repo/spacewave",
		BaseCommitHash: "1111111111111111111111111111111111111111",
	}})
	if err != nil {
		t.Fatal(err)
	}

	// Open a lazy cursor handle over the canonical read tree.
	cursor, err := NewFSCursor(newMemCursor(readRoot), resolver, allocator)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	// Register the mounted repository with the filesystem RPC server.
	srv := v86fs.NewServer(nil, nil)
	srv.AddMount("workspace", "/workspace", handle)
	mux := srpc.NewMux()
	if err := v86fs.SRPCRegisterV86FsService(mux, srv); err != nil {
		t.Fatal(err)
	}

	// Open a filesystem RPC relay through the in-memory server pipe.
	server := srpc.NewServer(mux)
	pipe := srpc.NewServerPipe(server)
	client := v86fs.NewSRPCV86FsServiceClient(srpc.NewClient(pipe))
	strm, err := client.RelayV86Fs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer strm.Close()

	// Mount the repository through the filesystem RPC client.
	tag := uint32(0)
	nextTag := func() uint32 { tag++; return tag }
	mountTag := nextTag()
	mountReply := sendV86fs(t, strm, &v86fs.V86FsMessage{
		Tag:  mountTag,
		Body: &v86fs.V86FsMessage_MountRequest{MountRequest: &v86fs.V86FsMountRequest{Name: "workspace"}},
	}).GetMountReply()

	// Verify that the filesystem mount request succeeded.
	if mountReply == nil || mountReply.GetStatus() != 0 {
		t.Fatalf("mount failed: %+v", mountReply)
	}

	// Resolve the read file inode through the mounted repository.
	rootID := mountReply.GetRootInodeId()
	reposID := lookupV86fs(t, strm, nextTag(), rootID, "repos")
	spacewaveID := lookupV86fs(t, strm, nextTag(), reposID, "spacewave")
	fileID := lookupV86fs(t, strm, nextTag(), spacewaveID, "README.md")

	// Write to the read inode through the filesystem RPC client.
	writeTag := nextTag()
	writeReply := sendV86fs(t, strm, &v86fs.V86FsMessage{
		Tag: writeTag,
		Body: &v86fs.V86FsMessage_WriteRequest{WriteRequest: &v86fs.V86FsWriteRequest{
			InodeId: fileID,
			Offset:  0,
			Data:    []byte("v86-write"),
		}},
	}).GetWriteReply()

	// Verify that the RPC write succeeds with one repository allocation.
	if writeReply == nil || writeReply.GetStatus() != 0 || writeReply.GetBytesWritten() != uint32(len("v86-write")) {
		t.Fatalf("write failed: %+v", writeReply)
	}
	if got := allocator.allocations.Load(); got != 1 {
		t.Fatalf("expected one allocation from v86fs write, got %d", got)
	}

	// Open the allocated file through its original inode.
	openTag := nextTag()
	openReply := sendV86fs(t, strm, &v86fs.V86FsMessage{
		Tag:  openTag,
		Body: &v86fs.V86FsMessage_OpenRequest{OpenRequest: &v86fs.V86FsOpenRequest{InodeId: fileID}},
	}).GetOpenReply()

	// Verify that the file open request succeeded.
	if openReply == nil || openReply.GetStatus() != 0 {
		t.Fatalf("open failed: %+v", openReply)
	}

	// Read the allocated file through the filesystem RPC client.
	readTag := nextTag()
	readReply := sendV86fs(t, strm, &v86fs.V86FsMessage{
		Tag: readTag,
		Body: &v86fs.V86FsMessage_ReadRequest{ReadRequest: &v86fs.V86FsReadRequest{
			HandleId: openReply.GetHandleId(),
			Size:     uint32(len("v86-write")),
		}},
	}).GetReadReply()

	// Verify the writable file content and preserved canonical read tree.
	if readReply == nil || readReply.GetStatus() != 0 {
		t.Fatalf("read failed: %+v", readReply)
	}
	if string(readReply.GetData()) != "v86-write" {
		t.Fatalf("expected v86fs read from writable tree, got %q", string(readReply.GetData()))
	}
	if string(readRoot.children["repos"].children["spacewave"].children["README.md"].data) != "read-mode" {
		t.Fatal("v86fs write mutated canonical read tree")
	}
}

type fakeAllocator struct {
	allocations atomic.Int64
	mtx         sync.Mutex
	open        func(ctx context.Context) (unixfs.FSCursor, error)
	onAllocate  func()
	requests    []AllocationRequest
}

func (a *fakeAllocator) AllocateWritableRepoTree(ctx context.Context, req AllocationRequest) (*AllocationResult, error) {
	// Count the repository allocation and invoke its optional gate.
	a.allocations.Add(1)
	if a.onAllocate != nil {
		a.onAllocate()
	}

	// Record the repository allocation request under the allocator lock.
	a.mtx.Lock()
	a.requests = append(a.requests, req)
	a.mtx.Unlock()

	return &AllocationResult{
		Allocation: &forge_lib_git_allocation.Allocation{
			ExecutionObjectKey: "forge/execution/test",
			RepoObjectKey:      req.RepoObjectKey,
			WorktreeObjectKey:  req.RepoObjectKey + "/worktree/test",
			BaseCommitHash:     req.BaseCommitHash,
			BranchRef:          "refs/heads/agent/test",
			PathFamily:         req.PathFamily,
			Status:             "allocated",
			CollisionState:     "none",
			StaleBaseState:     "current",
			CleanupState:       "active",
		},
		AllocationObjectKey: "forge/git/allocation/test",
		EvidenceObjectKey:   "evidence/allocation/test",
		OpenCursor:          a.open,
	}, nil
}

func (a *fakeAllocator) request(idx int) AllocationRequest {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return a.requests[idx]
}

func newLazyRepoTestHandle(t *testing.T) (*memNode, *memNode, *unixfs.FSHandle, *fakeAllocator) {
	// Prepare separate read and writable repository trees for the test.
	t.Helper()
	readRoot := newMemDir("", map[string]*memNode{
		"repos": newMemDir("repos", map[string]*memNode{
			"spacewave": newMemDir("spacewave", map[string]*memNode{
				"README.md": newMemFile("README.md", []byte("read-mode")),
			}),
		}),
	})
	writableRoot := newMemDir("", map[string]*memNode{
		"repos": newMemDir("repos", map[string]*memNode{
			"spacewave": newMemDir("spacewave", map[string]*memNode{
				"README.md": newMemFile("README.md", []byte("read-mode")),
			}),
		}),
	})

	// Configure the allocator to open the writable repository tree.
	allocator := &fakeAllocator{
		open: func(ctx context.Context) (unixfs.FSCursor, error) {
			return newMemCursor(writableRoot.children["repos"].children["spacewave"]), nil
		},
	}

	// Resolve the repository mount for lazy allocation.
	resolver, err := NewMountedRepoResolver([]RepoMount{{
		MountName:      "workspace",
		MountPath:      "/workspace",
		RepoRootPath:   "repos/spacewave",
		RepoObjectKey:  "repo/spacewave",
		BaseCommitHash: "1111111111111111111111111111111111111111",
	}})
	if err != nil {
		t.Fatal(err)
	}

	// Open a lazy cursor handle over the canonical read tree.
	cursor, err := NewFSCursor(newMemCursor(readRoot), resolver, allocator)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}

	return readRoot, writableRoot, handle, allocator
}

type memNode struct {
	mtx        sync.Mutex
	maxReadLen atomic.Int64
	name       string
	nodeType   unixfs.FSCursorNodeType
	data       []byte
	modTime    time.Time
	children   map[string]*memNode
}

func newMemDir(name string, children map[string]*memNode) *memNode {
	if children == nil {
		children = make(map[string]*memNode)
	}
	return &memNode{name: name, nodeType: unixfs.NewFSCursorNodeType_Dir(), children: children, modTime: time.Unix(1, 0)}
}

func newMemFile(name string, data []byte) *memNode {
	return &memNode{name: name, nodeType: unixfs.NewFSCursorNodeType_File(), data: slices.Clone(data), modTime: time.Unix(1, 0)}
}

type memCursor struct {
	released atomic.Bool
	node     *memNode
	parent   *memCursor
	path     []string
	root     *memNode
	cbs      unixfs.FSCursorChangeCbSlice
	mtx      sync.Mutex
}

func newMemCursor(root *memNode) *memCursor {
	return &memCursor{node: root, root: root}
}

func (c *memCursor) CheckReleased() bool {
	return c.released.Load()
}

func (c *memCursor) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	return nil, nil
}

func (c *memCursor) AddChangeCb(cb unixfs.FSCursorChangeCb) {
	c.mtx.Lock()
	if !c.CheckReleased() {
		c.cbs = append(c.cbs, cb)
		c.mtx.Unlock()
		return
	}
	c.mtx.Unlock()
	_ = cb(&unixfs.FSCursorChange{Cursor: c, Released: true})
}

func (c *memCursor) GetCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	if c.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	return &memOps{cursor: c}, nil
}

func (c *memCursor) Release() {
	// Claim the memory cursor release once.
	if c.released.Swap(true) {
		return
	}

	// Detach memory cursor callbacks under the cursor lock.
	c.mtx.Lock()
	cbs := c.cbs
	c.cbs = nil
	c.mtx.Unlock()

	// Notify the detached callbacks of memory cursor release.
	_ = cbs.CallCbs(&unixfs.FSCursorChange{Cursor: c, Released: true})
}

func (c *memCursor) child(name string, node *memNode) *memCursor {
	pathParts := make([]string, len(c.path), len(c.path)+1)
	copy(pathParts, c.path)
	pathParts = append(pathParts, name)
	return &memCursor{node: node, parent: c, path: pathParts, root: c.root}
}

type memOps struct {
	cursor *memCursor
}

func (o *memOps) CheckReleased() bool {
	return o.cursor.CheckReleased()
}

func (o *memOps) GetName() string {
	return o.cursor.node.name
}

func (o *memOps) GetIsDirectory() bool {
	return o.cursor.node.nodeType.GetIsDirectory()
}

func (o *memOps) GetIsFile() bool {
	return o.cursor.node.nodeType.GetIsFile()
}

func (o *memOps) GetIsSymlink() bool {
	return o.cursor.node.nodeType.GetIsSymlink()
}

func (o *memOps) GetPermissions(ctx context.Context) (fs.FileMode, error) {
	return unixfs.DefaultPermissions(o.cursor.node.nodeType), nil
}

func (o *memOps) SetPermissions(ctx context.Context, permissions fs.FileMode, ts time.Time) error {
	return nil
}

func (o *memOps) GetSize(ctx context.Context) (uint64, error) {
	if !o.GetIsFile() {
		return 0, nil
	}
	o.cursor.node.mtx.Lock()
	defer o.cursor.node.mtx.Unlock()
	return uint64(len(o.cursor.node.data)), nil
}

func (o *memOps) GetModTimestamp(ctx context.Context) (time.Time, error) {
	o.cursor.node.mtx.Lock()
	defer o.cursor.node.mtx.Unlock()
	return o.cursor.node.modTime, nil
}

func (o *memOps) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	o.cursor.node.mtx.Lock()
	defer o.cursor.node.mtx.Unlock()
	o.cursor.node.modTime = mtime
	return nil
}

func (o *memOps) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	// Require a file node before reading memory content.
	if !o.GetIsFile() {
		return 0, unixfs_errors.ErrNotFile
	}

	// Lock the memory file for the read operation.
	o.cursor.node.mtx.Lock()
	defer o.cursor.node.mtx.Unlock()

	// Record the largest read request for the streamed copy assertion.
	for {
		prev := o.cursor.node.maxReadLen.Load()
		if int64(len(data)) <= prev || o.cursor.node.maxReadLen.CompareAndSwap(prev, int64(len(data))) {
			break
		}
	}

	// Stop the memory read at the end of the file.
	if offset >= int64(len(o.cursor.node.data)) {
		return 0, io.EOF
	}

	// Copy the available memory file content into the read buffer.
	n := copy(data, o.cursor.node.data[offset:])
	if n < len(data) {
		return int64(n), io.EOF
	}

	return int64(n), nil
}

func (o *memOps) GetOptimalWriteSize(ctx context.Context) (int64, error) {
	return 0, nil
}

func (o *memOps) WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	// Require a file node before writing memory content.
	if !o.GetIsFile() {
		return unixfs_errors.ErrNotFile
	}

	// Lock the memory file for the write operation.
	o.cursor.node.mtx.Lock()
	defer o.cursor.node.mtx.Unlock()

	// Grow the memory file to contain the requested write.
	end := int(offset) + len(data)
	if end > len(o.cursor.node.data) {
		next := make([]byte, end)
		copy(next, o.cursor.node.data)
		o.cursor.node.data = next
	}

	// Write the memory file content and record its modification time.
	copy(o.cursor.node.data[offset:], data)
	o.cursor.node.modTime = ts

	return nil
}

func (o *memOps) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	// Require a file node before truncating memory content.
	if !o.GetIsFile() {
		return unixfs_errors.ErrNotFile
	}

	// Resize the locked memory file and record its modification time.
	o.cursor.node.mtx.Lock()
	defer o.cursor.node.mtx.Unlock()
	next := make([]byte, nsize)
	copy(next, o.cursor.node.data)
	o.cursor.node.data = next
	o.cursor.node.modTime = ts

	return nil
}

func (o *memOps) Lookup(ctx context.Context, name string) (unixfs.FSCursor, error) {
	// Require a directory node before looking up a memory child.
	if !o.GetIsDirectory() {
		return nil, unixfs_errors.ErrNotDirectory
	}

	// Resolve the memory child under the directory lock.
	o.cursor.node.mtx.Lock()
	defer o.cursor.node.mtx.Unlock()
	child := o.cursor.node.children[name]
	if child == nil {
		return nil, unixfs_errors.ErrNotExist
	}

	return o.cursor.child(name, child), nil
}

func (o *memOps) ReaddirAll(ctx context.Context, skip uint64, cb func(ent unixfs.FSCursorDirent) error) error {
	// Require a directory node before listing memory children.
	if !o.GetIsDirectory() {
		return unixfs_errors.ErrNotDirectory
	}

	// Snapshot memory children in sorted name order under the directory lock.
	o.cursor.node.mtx.Lock()
	names := make([]string, 0, len(o.cursor.node.children))
	for name := range o.cursor.node.children {
		names = append(names, name)
	}
	slices.Sort(names)
	children := make([]*memNode, 0, len(names))
	for _, name := range names {
		children = append(children, o.cursor.node.children[name])
	}
	o.cursor.node.mtx.Unlock()

	// Deliver the requested memory directory entries outside the lock.
	for idx := range names {
		if uint64(idx) < skip {
			continue
		}
		if err := cb(memDirent{node: children[idx]}); err != nil {
			return err
		}
	}

	return nil
}

func (o *memOps) Mknod(ctx context.Context, checkExist bool, names []string, nodeType unixfs.FSCursorNodeType, permissions fs.FileMode, ts time.Time) error {
	o.cursor.node.mtx.Lock()
	defer o.cursor.node.mtx.Unlock()
	for _, name := range names {
		if checkExist && o.cursor.node.children[name] != nil {
			return unixfs_errors.ErrExist
		}
		if nodeType.GetIsDirectory() {
			o.cursor.node.children[name] = newMemDir(name, nil)
		} else {
			o.cursor.node.children[name] = newMemFile(name, nil)
		}
	}
	return nil
}

func (o *memOps) Symlink(ctx context.Context, checkExist bool, name string, target []string, targetIsAbsolute bool, ts time.Time) error {
	return unixfs_errors.ErrNotSymlink
}

func (o *memOps) Readlink(ctx context.Context, name string) ([]string, bool, error) {
	return nil, false, unixfs_errors.ErrNotSymlink
}

func (o *memOps) CopyTo(ctx context.Context, tgtDir unixfs.FSCursorOps, tgtName string, ts time.Time) (bool, error) {
	return false, nil
}

func (o *memOps) CopyFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (bool, error) {
	return false, nil
}

func (o *memOps) MoveTo(ctx context.Context, tgtCursorOps unixfs.FSCursorOps, tgtName string, ts time.Time) (bool, error) {
	return false, nil
}

func (o *memOps) MoveFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (bool, error) {
	return false, nil
}

func (o *memOps) Remove(ctx context.Context, names []string, ts time.Time) error {
	o.cursor.node.mtx.Lock()
	defer o.cursor.node.mtx.Unlock()
	for _, name := range names {
		delete(o.cursor.node.children, name)
	}
	return nil
}

func (o *memOps) MknodWithContent(ctx context.Context, name string, nodeType unixfs.FSCursorNodeType, dataLen int64, rdr io.Reader, permissions fs.FileMode, ts time.Time) error {
	// Read the source stream for the new memory file.
	data, err := io.ReadAll(rdr)
	if err != nil {
		return err
	}

	// Create the memory child under the directory lock.
	o.cursor.node.mtx.Lock()
	defer o.cursor.node.mtx.Unlock()
	o.cursor.node.children[name] = newMemFile(name, data)

	return nil
}

type memDirent struct {
	node *memNode
}

func (d memDirent) GetName() string {
	return d.node.name
}

func (d memDirent) GetIsDirectory() bool {
	return d.node.nodeType.GetIsDirectory()
}

func (d memDirent) GetIsFile() bool {
	return d.node.nodeType.GetIsFile()
}

func (d memDirent) GetIsSymlink() bool {
	return d.node.nodeType.GetIsSymlink()
}

var (
	_ unixfs.FSCursor       = (*memCursor)(nil)
	_ unixfs.FSCursorOps    = (*memOps)(nil)
	_ unixfs.FSCursorDirent = (*memDirent)(nil)
)

func lookupV86fs(t *testing.T, strm v86fs.SRPCV86FsService_RelayV86FsClient, tag uint32, parentID uint64, name string) uint64 {
	t.Helper()
	reply := sendV86fs(t, strm, &v86fs.V86FsMessage{
		Tag: tag,
		Body: &v86fs.V86FsMessage_LookupRequest{LookupRequest: &v86fs.V86FsLookupRequest{
			ParentId: parentID,
			Name:     name,
		}},
	}).GetLookupReply()
	if reply == nil || reply.GetStatus() != 0 || reply.GetInodeId() == 0 {
		t.Fatalf("lookup %q failed: %+v", name, reply)
	}
	return reply.GetInodeId()
}

func sendV86fs(t *testing.T, strm v86fs.SRPCV86FsService_RelayV86FsClient, msg *v86fs.V86FsMessage) *v86fs.V86FsMessage {
	// Send the filesystem RPC request on the relay stream.
	t.Helper()
	if err := strm.Send(msg); err != nil {
		t.Fatal(err)
	}

	// Read relay messages until the matching filesystem reply arrives.
	for range 20 {
		reply, err := strm.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if reply.GetTag() == msg.GetTag() {
			return reply
		}
	}

	// Fail the test when the filesystem relay yields no matching reply.
	t.Fatalf("no reply for v86fs tag %d", msg.GetTag())
	return nil
}
