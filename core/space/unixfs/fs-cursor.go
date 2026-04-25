package space_unixfs

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// FSCursor projects one space into a lazy filesystem rooted at u/{idx}/so/{soId}.
type FSCursor struct {
	// le is the logger used when opening projected object cursors.
	le *logrus.Entry
	// ws is the world state for the mounted space.
	ws world.WorldState
	// sessionIdx is the projected session index segment.
	sessionIdx uint32
	// sharedObjectID is the projected shared object identifier segment.
	sharedObjectID string
	// path is the projected path from the synthetic root.
	path []string

	// mtx guards proxyCursor.
	mtx sync.Mutex
	// proxyCursor is the lazily-opened object cursor for exact object paths.
	proxyCursor unixfs.FSCursor

	// released indicates if the cursor has been released.
	released atomic.Bool
}

// NewFSCursor constructs a projected cursor rooted at u/{idx}/so/{soId}.
func NewFSCursor(
	le *logrus.Entry,
	ws world.WorldState,
	sessionIdx uint32,
	sharedObjectID string,
) *FSCursor {
	return &FSCursor{
		le:             le,
		ws:             ws,
		sessionIdx:     sessionIdx,
		sharedObjectID: sharedObjectID,
	}
}

// CheckReleased checks if the cursor is released.
func (f *FSCursor) CheckReleased() bool {
	return f.released.Load()
}

// GetProxyCursor returns the mounted object cursor for exact object paths.
func (f *FSCursor) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	if f.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}

	objectKey, ok, err := f.getMountedObjectKey(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	f.mtx.Lock()
	defer f.mtx.Unlock()

	if f.released.Load() {
		return nil, unixfs_errors.ErrReleased
	}
	if f.proxyCursor != nil && !f.proxyCursor.CheckReleased() {
		return f.proxyCursor, nil
	}

	cursor, err := openObjectCursor(ctx, f.le, f.ws, objectKey)
	if err != nil {
		return nil, err
	}
	f.proxyCursor = cursor
	return cursor, nil
}

// AddChangeCb adds a change callback.
func (f *FSCursor) AddChangeCb(cb unixfs.FSCursorChangeCb) {}

// GetCursorOps returns the synthetic directory ops for the projected cursor.
func (f *FSCursor) GetCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	if f.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	return &fsCursorOps{cursor: f}, nil
}

// Release releases the cursor and any mounted object cursor.
func (f *FSCursor) Release() {
	if f.released.Swap(true) {
		return
	}

	f.mtx.Lock()
	defer f.mtx.Unlock()

	if f.proxyCursor != nil {
		f.proxyCursor.Release()
		f.proxyCursor = nil
	}
}

func (f *FSCursor) getName() string {
	if len(f.path) == 0 {
		return ""
	}
	return f.path[len(f.path)-1]
}

func (f *FSCursor) buildChild(name string) *FSCursor {
	path := make([]string, len(f.path)+1)
	copy(path, f.path)
	path[len(path)-1] = name
	return &FSCursor{
		le:             f.le,
		ws:             f.ws,
		sessionIdx:     f.sessionIdx,
		sharedObjectID: f.sharedObjectID,
		path:           path,
	}
}

func (f *FSCursor) getProjectionTail() ([]string, bool) {
	if len(f.path) < 5 {
		return nil, false
	}
	if f.path[0] != "u" || f.path[1] != strconv.FormatUint(uint64(f.sessionIdx), 10) {
		return nil, false
	}
	if f.path[2] != "so" || f.path[3] != f.sharedObjectID || f.path[4] != "-" {
		return nil, false
	}
	return f.path[5:], true
}

func (f *FSCursor) getMountedObjectKey(ctx context.Context) (string, bool, error) {
	tail, ok := f.getProjectionTail()
	if !ok || len(tail) == 0 || tail[len(tail)-1] != "-" {
		return "", false, nil
	}

	objectPath := tail[:len(tail)-1]
	if len(objectPath) == 0 {
		return "", false, nil
	}

	objects, err := listProjectedObjects(ctx, f.ws)
	if err != nil {
		return "", false, err
	}
	objectKey, found := findExactObjectKey(objectPath, objects)
	return objectKey, found, nil
}

func (f *FSCursor) lookupChild(ctx context.Context, name string) (unixfs.FSCursor, error) {
	if f.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}

	switch len(f.path) {
	case 0:
		if name != "u" {
			return nil, unixfs_errors.ErrNotExist
		}
		return f.buildChild(name), nil
	case 1:
		if name != strconv.FormatUint(uint64(f.sessionIdx), 10) {
			return nil, unixfs_errors.ErrNotExist
		}
		return f.buildChild(name), nil
	case 2:
		if name != "so" {
			return nil, unixfs_errors.ErrNotExist
		}
		return f.buildChild(name), nil
	case 3:
		if name != f.sharedObjectID {
			return nil, unixfs_errors.ErrNotExist
		}
		return f.buildChild(name), nil
	case 4:
		if name != "-" {
			return nil, unixfs_errors.ErrNotExist
		}
		return f.buildChild(name), nil
	}

	if _, ok, err := f.getMountedObjectKey(ctx); err != nil {
		return nil, err
	} else if ok {
		return nil, unixfs_errors.ErrNotExist
	}

	children, err := f.listChildren(ctx)
	if err != nil {
		return nil, err
	}
	if _, ok := children[name]; !ok {
		return nil, unixfs_errors.ErrNotExist
	}
	return f.buildChild(name), nil
}

func (f *FSCursor) listChildren(ctx context.Context) (map[string]*projectedChild, error) {
	path, ok := f.getProjectionTail()
	if !ok {
		return nil, unixfs_errors.ErrNotExist
	}
	if len(path) > 0 && path[len(path)-1] == "-" {
		return nil, unixfs_errors.ErrNotDirectory
	}

	objects, err := listProjectedObjects(ctx, f.ws)
	if err != nil {
		return nil, err
	}

	children := make(map[string]*projectedChild)
	for _, objectKey := range objects {
		segs := splitObjectKey(objectKey)
		if !hasPathPrefix(segs, path) || len(segs) <= len(path) {
			continue
		}

		childName := segs[len(path)]
		children[childName] = &projectedChild{name: childName}
	}

	if len(path) > 0 {
		if _, found := findExactObjectKey(path, objects); found {
			children["-"] = &projectedChild{name: "-"}
		}
	}

	return children, nil
}

func (f *FSCursor) readdirChildren(ctx context.Context, skip uint64, cb func(ent unixfs.FSCursorDirent) error) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	var dirents []*projectedDirent
	switch len(f.path) {
	case 0:
		dirents = append(dirents, newProjectedDirent("u", unixfs.NewFSCursorNodeType_Dir()))
	case 1:
		dirents = append(dirents, newProjectedDirent(strconv.FormatUint(uint64(f.sessionIdx), 10), unixfs.NewFSCursorNodeType_Dir()))
	case 2:
		dirents = append(dirents, newProjectedDirent("so", unixfs.NewFSCursorNodeType_Dir()))
	case 3:
		dirents = append(dirents, newProjectedDirent(f.sharedObjectID, unixfs.NewFSCursorNodeType_Dir()))
	case 4:
		dirents = append(dirents, newProjectedDirent("-", unixfs.NewFSCursorNodeType_Dir()))
	default:
		children, err := f.listChildren(ctx)
		if err != nil {
			return err
		}
		dirents = buildProjectedDirents(children)
	}

	for i, dirent := range dirents {
		if uint64(i) < skip {
			continue
		}
		if err := cb(dirent); err != nil {
			return err
		}
	}
	return nil
}

// _ is a type assertion
var _ unixfs.FSCursor = ((*FSCursor)(nil))
