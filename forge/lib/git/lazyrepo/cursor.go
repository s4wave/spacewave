package forge_lib_git_lazyrepo

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	forge_lib_git_allocation "github.com/s4wave/spacewave/forge/lib/git/allocation"
)

// Allocator materializes a writable cursor tree for a repo root on first write.
type Allocator interface {
	AllocateWritableRepoTree(ctx context.Context, req AllocationRequest) (*AllocationResult, error)
}

// AllocationRequest is passed to Allocator before a repo-root mutation.
type AllocationRequest struct {
	ResolvedPath
	// Operation is the mutation operation that triggered allocation.
	Operation string
}

// AllocationResult returns allocation provenance and a way to open writable cursors.
type AllocationResult struct {
	// Allocation is the Forge/Git allocation record when one was persisted.
	Allocation *forge_lib_git_allocation.Allocation
	// AllocationObjectKey is the allocation object key.
	AllocationObjectKey string
	// EvidenceObjectKey points at raw provider Evidence when present.
	EvidenceObjectKey string
	// OpenCursor opens a fresh writable cursor at the allocated repo root.
	OpenCursor func(ctx context.Context) (unixfs.FSCursor, error)
}

type allocatedRoot struct {
	resolved ResolvedPath
	result   *AllocationResult
}

type allocationSlot struct {
	ready chan struct{}
	alloc *allocatedRoot
	err   error
}

type lazyTree struct {
	mtx         sync.Mutex
	resolver    *MountedRepoResolver
	allocator   Allocator
	allocations map[string]*allocationSlot
}

// FSCursor is a lazy writable Repo tree cursor.
type FSCursor struct {
	released atomic.Bool
	tree     *lazyTree
	base     unixfs.FSCursor
	parent   *FSCursor
	name     string
	path     []string

	mtx      sync.Mutex
	cbs      unixfs.FSCursorChangeCbSlice
	children []*FSCursor
}

// NewFSCursor wraps a canonical read-mode cursor with lazy repo allocation.
func NewFSCursor(base unixfs.FSCursor, resolver *MountedRepoResolver, allocator Allocator) (*FSCursor, error) {
	if base == nil {
		return nil, errors.New("base cursor cannot be nil")
	}
	if resolver == nil {
		return nil, errors.New("resolver cannot be nil")
	}
	if allocator == nil {
		return nil, errors.New("allocator cannot be nil")
	}
	return &FSCursor{
		tree: &lazyTree{
			resolver:    resolver,
			allocator:   allocator,
			allocations: make(map[string]*allocationSlot),
		},
		base: base,
	}, nil
}

// CheckReleased checks if the cursor was released.
func (c *FSCursor) CheckReleased() bool {
	return c == nil || c.released.Load() || c.base.CheckReleased()
}

// GetProxyCursor returns a writable cursor when this path has already allocated.
func (c *FSCursor) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	if c.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	alloc := c.lookupAllocation()
	if alloc == nil {
		return nil, nil
	}
	return c.openWritableCursor(ctx, alloc)
}

// AddChangeCb registers a cursor change callback.
func (c *FSCursor) AddChangeCb(cb unixfs.FSCursorChangeCb) {
	if cb == nil {
		return
	}
	c.mtx.Lock()
	if !c.CheckReleased() {
		c.cbs = append(c.cbs, cb)
		c.mtx.Unlock()
		return
	}
	c.mtx.Unlock()
	_ = cb(&unixfs.FSCursorChange{Cursor: c, Released: true})
}

// GetCursorOps returns lazy repo cursor operations.
func (c *FSCursor) GetCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	if c.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	ops, err := resolveCursorOps(ctx, c.base)
	if err != nil {
		return nil, err
	}
	return &FSCursorOps{cursor: c, base: ops}, nil
}

// Release releases this cursor and its read-mode children.
func (c *FSCursor) Release() {
	if c == nil || c.released.Swap(true) {
		return
	}
	c.releaseChildren()
	c.base.Release()
	c.mtx.Lock()
	cbs := c.cbs
	c.cbs = nil
	c.mtx.Unlock()
	_ = cbs.CallCbs(&unixfs.FSCursorChange{Cursor: c, Released: true})
}

func (c *FSCursor) child(name string, base unixfs.FSCursor) *FSCursor {
	pathParts := make([]string, len(c.path), len(c.path)+1)
	copy(pathParts, c.path)
	pathParts = append(pathParts, name)
	child := &FSCursor{
		tree:   c.tree,
		base:   base,
		parent: c,
		name:   name,
		path:   pathParts,
	}
	c.mtx.Lock()
	if c.CheckReleased() {
		c.mtx.Unlock()
		child.Release()
		return child
	}
	c.children = append(c.children, child)
	c.mtx.Unlock()
	return child
}

func (c *FSCursor) ensureWritable(ctx context.Context, operation string, mutationPath []string) (*allocatedRoot, bool, error) {
	resolved, err := c.tree.resolver.ResolveCursorPath(mutationPath)
	if err != nil {
		if perr, ok := err.(*ProvenanceError); ok {
			perr.Operation = operation
		}
		return nil, false, err
	}
	key := allocationKey(resolved)

	c.tree.mtx.Lock()
	if slot := c.tree.allocations[key]; slot != nil {
		c.tree.mtx.Unlock()
		select {
		case <-ctx.Done():
			return nil, false, context.Canceled
		case <-slot.ready:
		}
		return slot.alloc, false, slot.err
	}
	slot := &allocationSlot{ready: make(chan struct{})}
	c.tree.allocations[key] = slot
	c.tree.mtx.Unlock()

	result, err := c.tree.allocator.AllocateWritableRepoTree(ctx, AllocationRequest{
		ResolvedPath: resolved,
		Operation:    operation,
	})
	if err == nil && (result == nil || result.OpenCursor == nil) {
		err = errors.New("allocator returned no writable cursor opener")
	}

	c.tree.mtx.Lock()
	if err != nil {
		delete(c.tree.allocations, key)
		slot.err = err
	} else {
		slot.alloc = &allocatedRoot{
			resolved: resolved,
			result:   result,
		}
	}
	c.tree.mtx.Unlock()
	close(slot.ready)
	if err != nil {
		return nil, false, err
	}

	rootCursor := c.findRepoRootCursor(resolved.RepoRootPath)
	if rootCursor != nil {
		rootCursor.releaseChildren()
	}
	return slot.alloc, true, nil
}

func (c *FSCursor) lookupAllocation() *allocatedRoot {
	c.tree.mtx.Lock()
	defer c.tree.mtx.Unlock()
	var best *allocatedRoot
	bestLen := -1
	for _, slot := range c.tree.allocations {
		select {
		case <-slot.ready:
		default:
			continue
		}
		alloc := slot.alloc
		if alloc == nil || slot.err != nil {
			continue
		}
		rootParts, _ := unixfs.CleanSplitValidateRelativePath(alloc.resolved.RepoRootPath)
		if !hasPathPrefix(c.path, rootParts) || len(rootParts) <= bestLen {
			continue
		}
		best = alloc
		bestLen = len(rootParts)
	}
	return best
}

func (c *FSCursor) openWritableCursor(ctx context.Context, alloc *allocatedRoot) (unixfs.FSCursor, error) {
	handle, err := c.openWritableHandle(ctx, alloc)
	if err != nil {
		return nil, err
	}
	return unixfs.NewFSHandleCursor(handle, true, nil), nil
}

func (c *FSCursor) openWritableHandle(ctx context.Context, alloc *allocatedRoot) (*unixfs.FSHandle, error) {
	rootCursor, err := alloc.result.OpenCursor(ctx)
	if err != nil {
		return nil, err
	}
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		return nil, err
	}
	rootParts, err := unixfs.CleanSplitValidateRelativePath(alloc.resolved.RepoRootPath)
	if err != nil {
		rootHandle.Release()
		return nil, err
	}
	relPath := c.path[len(rootParts):]
	if len(relPath) == 0 {
		return rootHandle, nil
	}
	writableHandle, _, err := rootHandle.LookupPathPts(ctx, relPath)
	rootHandle.Release()
	if err != nil {
		return nil, err
	}
	return writableHandle, nil
}

func (c *FSCursor) findRepoRootCursor(repoRootPath string) *FSCursor {
	rootParts, err := unixfs.CleanSplitValidateRelativePath(repoRootPath)
	if err != nil {
		return nil
	}
	for cur := c; cur != nil; cur = cur.parent {
		if slices.Equal(cur.path, rootParts) {
			return cur
		}
	}
	return nil
}

func (c *FSCursor) releaseChildren() {
	c.mtx.Lock()
	children := c.children
	c.children = nil
	c.mtx.Unlock()
	for _, child := range children {
		child.Release()
	}
}

func resolveCursorOps(ctx context.Context, cursor unixfs.FSCursor) (unixfs.FSCursorOps, error) {
	for range 100 {
		if cursor.CheckReleased() {
			return nil, unixfs_errors.ErrReleased
		}
		proxy, err := cursor.GetProxyCursor(ctx)
		if err != nil {
			return nil, err
		}
		if proxy != nil {
			cursor = proxy
			continue
		}
		ops, err := cursor.GetCursorOps(ctx)
		if err != nil {
			return nil, err
		}
		if ops == nil {
			return nil, unixfs_errors.ErrNotExist
		}
		return ops, nil
	}
	return nil, unixfs_errors.ErrInodeUnresolvable
}

var _ unixfs.FSCursor = (*FSCursor)(nil)
