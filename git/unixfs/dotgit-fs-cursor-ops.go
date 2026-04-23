package unixfs_git

import (
	"context"
	"io"
	"io/fs"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	go_git_storer "github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/pkg/errors"
)

const dotGitDefaultDescription = "Unnamed repository; edit this file 'description' to name the repository.\n"

// DotGitFSCursorOps implements unixfs.FSCursorOps for .git layout nodes.
type DotGitFSCursorOps struct {
	isReleased atomic.Bool
	cursor     *DotGitFSCursor
	node       *dotGitNode
}

func newDotGitFSCursorOps(c *DotGitFSCursor) *DotGitFSCursorOps {
	return &DotGitFSCursorOps{
		cursor: c,
		node:   c.node,
	}
}

// CheckReleased checks if the ops is released.
func (o *DotGitFSCursorOps) CheckReleased() bool {
	if o == nil {
		return true
	}
	return o.isReleased.Load() || o.cursor.CheckReleased()
}

// GetName returns the name of the node.
func (o *DotGitFSCursorOps) GetName() string {
	return o.node.name
}

// GetIsDirectory returns if the node is a directory.
func (o *DotGitFSCursorOps) GetIsDirectory() bool {
	return o.node.kind == dotGitNodeKindDir
}

// GetIsFile returns if the node is a regular file.
func (o *DotGitFSCursorOps) GetIsFile() bool {
	return o.node.kind == dotGitNodeKindFile
}

// GetIsSymlink returns false because .git seed nodes are not symlinks.
func (o *DotGitFSCursorOps) GetIsSymlink() bool {
	return false
}

// GetPermissions returns the permissions for this node.
func (o *DotGitFSCursorOps) GetPermissions(ctx context.Context) (fs.FileMode, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	if o.GetIsDirectory() {
		return 0o755 | fs.ModeDir, nil
	}
	return 0o644, nil
}

// SetPermissions returns ErrReadOnly.
func (o *DotGitFSCursorOps) SetPermissions(ctx context.Context, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// GetSize returns the node size in bytes.
func (o *DotGitFSCursorOps) GetSize(ctx context.Context) (uint64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	if o.GetIsDirectory() {
		return 0, nil
	}
	content, err := o.content(ctx)
	if err != nil {
		return 0, err
	}
	return uint64(len(content)), nil
}

// GetModTimestamp returns the modification timestamp.
func (o *DotGitFSCursorOps) GetModTimestamp(ctx context.Context) (time.Time, error) {
	if o.CheckReleased() {
		return time.Time{}, unixfs_errors.ErrReleased
	}
	return time.Time{}, nil
}

// SetModTimestamp returns ErrReadOnly.
func (o *DotGitFSCursorOps) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// ReadAt reads file content from the node.
func (o *DotGitFSCursorOps) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	if !o.GetIsFile() {
		return 0, unixfs_errors.ErrNotFile
	}
	if offset < 0 {
		return 0, errors.New("negative offset")
	}
	content, err := o.content(ctx)
	if err != nil {
		return 0, err
	}
	if offset >= int64(len(content)) {
		return 0, io.EOF
	}
	n := copy(data, content[offset:])
	if n < len(data) {
		return int64(n), io.EOF
	}
	return int64(n), nil
}

// GetOptimalWriteSize returns 0, ErrReadOnly.
func (o *DotGitFSCursorOps) GetOptimalWriteSize(ctx context.Context) (int64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return 0, unixfs_errors.ErrReadOnly
}

// WriteAt returns ErrReadOnly.
func (o *DotGitFSCursorOps) WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Truncate returns ErrReadOnly.
func (o *DotGitFSCursorOps) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Lookup looks up a child entry in a directory.
func (o *DotGitFSCursorOps) Lookup(ctx context.Context, name string) (unixfs.FSCursor, error) {
	if o.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	if !o.GetIsDirectory() {
		return nil, unixfs_errors.ErrNotDirectory
	}
	if child, ok, err := o.lookupRef(name); ok || err != nil {
		if err != nil {
			return nil, err
		}
		return newDotGitFSCursorFromNode(o.cursor.storer, child), nil
	}
	child := o.node.child(name)
	if child == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	return newDotGitFSCursorFromNode(o.cursor.storer, child), nil
}

// ReaddirAll reads all directory entries.
func (o *DotGitFSCursorOps) ReaddirAll(ctx context.Context, skip uint64, cb func(ent unixfs.FSCursorDirent) error) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.GetIsDirectory() {
		return unixfs_errors.ErrNotDirectory
	}
	if ents, ok, err := o.readRefsDir(); ok || err != nil {
		if err != nil {
			return err
		}
		for i := int(skip); i < len(ents); i++ { //nolint:gosec
			if err := cb(ents[i]); err != nil {
				return err
			}
		}
		return nil
	}
	for i := int(skip); i < len(o.node.children); i++ { //nolint:gosec
		child := o.node.children[i]
		ent := &gitDirent{
			name:   child.name,
			isDir:  child.kind == dotGitNodeKindDir,
			isFile: child.kind == dotGitNodeKindFile,
		}
		if err := cb(ent); err != nil {
			return err
		}
	}
	return nil
}

// Mknod returns ErrReadOnly.
func (o *DotGitFSCursorOps) Mknod(ctx context.Context, checkExist bool, names []string, nodeType unixfs.FSCursorNodeType, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Symlink returns ErrReadOnly.
func (o *DotGitFSCursorOps) Symlink(ctx context.Context, checkExist bool, name string, target []string, tgtIsAbsolute bool, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Readlink returns ErrNotSymlink.
func (o *DotGitFSCursorOps) Readlink(ctx context.Context, name string) ([]string, bool, error) {
	if o.CheckReleased() {
		return nil, false, unixfs_errors.ErrReleased
	}
	return nil, false, unixfs_errors.ErrNotSymlink
}

// CopyTo returns false, nil.
func (o *DotGitFSCursorOps) CopyTo(ctx context.Context, tgtDir unixfs.FSCursorOps, tgtName string, ts time.Time) (bool, error) {
	return false, nil
}

// CopyFrom returns false, nil.
func (o *DotGitFSCursorOps) CopyFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (bool, error) {
	return false, nil
}

// MoveTo returns false, ErrReadOnly.
func (o *DotGitFSCursorOps) MoveTo(ctx context.Context, tgtCursorOps unixfs.FSCursorOps, tgtName string, ts time.Time) (bool, error) {
	return false, unixfs_errors.ErrReadOnly
}

// MoveFrom returns false, ErrReadOnly.
func (o *DotGitFSCursorOps) MoveFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (bool, error) {
	return false, unixfs_errors.ErrReadOnly
}

// Remove returns ErrReadOnly.
func (o *DotGitFSCursorOps) Remove(ctx context.Context, names []string, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// MknodWithContent returns ErrReadOnly.
func (o *DotGitFSCursorOps) MknodWithContent(ctx context.Context, name string, nodeType unixfs.FSCursorNodeType, dataLen int64, rdr io.Reader, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

func (o *DotGitFSCursorOps) content(ctx context.Context) ([]byte, error) {
	switch o.node.name {
	case "HEAD":
		return dotGitHeadContent(o.cursor.storer)
	case "config":
		return dotGitConfigContent(o.cursor.storer)
	case "description":
		return []byte(dotGitDefaultDescription), nil
	default:
		return o.node.content, nil
	}
}

func (o *DotGitFSCursorOps) lookupRef(name string) (*dotGitNode, bool, error) {
	switch {
	case slices.Equal(o.node.path, []string{"refs"}):
		switch name {
		case "heads":
			return newDotGitDirNode(name, []string{"refs", "heads"}), true, nil
		case "tags":
			return newDotGitDirNode(name, []string{"refs", "tags"}), true, nil
		default:
			return nil, true, unixfs_errors.ErrNotExist
		}
	case dotGitRefsPathKind(o.node.path) != "":
		return o.lookupRefBelowKind(name)
	default:
		return nil, false, nil
	}
}

func (o *DotGitFSCursorOps) lookupRefBelowKind(name string) (*dotGitNode, bool, error) {
	kind := dotGitRefsPathKind(o.node.path)
	prefix := o.node.path[2:]
	refs, err := dotGitCollectRefs(o.cursor.storer, kind, prefix)
	if err != nil {
		return nil, true, err
	}
	for _, ref := range refs {
		parts, ok := dotGitReferencePath(ref, kind)
		if !ok || len(parts) <= len(prefix) || !slices.Equal(parts[:len(prefix)], prefix) {
			continue
		}
		next := parts[len(prefix)]
		if next != name {
			continue
		}
		path := append(append([]string{}, o.node.path...), name)
		if len(parts) == len(prefix)+1 {
			content := []byte(dotGitReferenceFileContent(ref))
			return newDotGitFileNode(name, path, content), true, nil
		}
		return newDotGitDirNode(name, path), true, nil
	}
	return nil, true, unixfs_errors.ErrNotExist
}

func (o *DotGitFSCursorOps) readRefsDir() ([]unixfs.FSCursorDirent, bool, error) {
	switch {
	case slices.Equal(o.node.path, []string{"refs"}):
		return []unixfs.FSCursorDirent{
			&gitDirent{name: "heads", isDir: true},
			&gitDirent{name: "tags", isDir: true},
		}, true, nil
	case dotGitRefsPathKind(o.node.path) != "":
		ents, err := o.readRefKindDir()
		return ents, true, err
	default:
		return nil, false, nil
	}
}

func (o *DotGitFSCursorOps) readRefKindDir() ([]unixfs.FSCursorDirent, error) {
	kind := dotGitRefsPathKind(o.node.path)
	prefix := o.node.path[2:]
	refs, err := dotGitCollectRefs(o.cursor.storer, kind, prefix)
	if err != nil {
		return nil, err
	}
	type ent struct {
		isDir bool
	}
	seen := make(map[string]ent)
	for _, ref := range refs {
		parts, ok := dotGitReferencePath(ref, kind)
		if !ok || len(parts) <= len(prefix) || !slices.Equal(parts[:len(prefix)], prefix) {
			continue
		}
		name := parts[len(prefix)]
		prev := seen[name]
		if len(parts) > len(prefix)+1 {
			prev.isDir = true
		}
		seen[name] = prev
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	ents := make([]unixfs.FSCursorDirent, 0, len(names))
	for _, name := range names {
		info := seen[name]
		ents = append(ents, &gitDirent{
			name:   name,
			isDir:  info.isDir,
			isFile: !info.isDir,
		})
	}
	return ents, nil
}

func dotGitHeadContent(storer interface {
	Reference(plumbing.ReferenceName) (*plumbing.Reference, error)
}) ([]byte, error) {
	if storer == nil {
		return []byte("ref: refs/heads/master\n"), nil
	}
	ref, err := storer.Reference(plumbing.HEAD)
	if err == plumbing.ErrReferenceNotFound {
		return []byte("ref: refs/heads/master\n"), nil
	}
	if err != nil {
		return nil, err
	}
	parts := ref.Strings()
	return []byte(parts[1] + "\n"), nil
}

func dotGitConfigContent(storer interface {
	Config() (*config.Config, error)
}) ([]byte, error) {
	cfg := config.NewConfig()
	if storer != nil {
		var err error
		cfg, err = storer.Config()
		if err != nil {
			return nil, err
		}
	}
	return cfg.Marshal()
}

func dotGitReferenceFileContent(ref *plumbing.Reference) string {
	parts := ref.Strings()
	return parts[1] + "\n"
}

func dotGitRefsPathKind(path []string) string {
	if len(path) < 2 || path[0] != "refs" {
		return ""
	}
	switch path[1] {
	case "heads":
		return "heads"
	case "tags":
		return "tags"
	default:
		return ""
	}
}

func dotGitCollectRefs(refStorer interface {
	IterReferences() (go_git_storer.ReferenceIter, error)
}, kind string, prefix []string) ([]*plumbing.Reference, error) {
	if refStorer == nil {
		return nil, nil
	}
	iter, err := refStorer.IterReferences()
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var refs []*plumbing.Reference
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		parts, ok := dotGitReferencePath(ref, kind)
		if ok && len(parts) > len(prefix) && slices.Equal(parts[:len(prefix)], prefix) {
			refs = append(refs, ref)
		}
		return nil
	})
	return refs, err
}

func dotGitReferencePath(ref *plumbing.Reference, kind string) ([]string, bool) {
	var prefix string
	switch kind {
	case "heads":
		prefix = "refs/heads/"
	case "tags":
		prefix = "refs/tags/"
	default:
		return nil, false
	}
	name := ref.Name().String()
	if !strings.HasPrefix(name, prefix) {
		return nil, false
	}
	rest := strings.TrimPrefix(name, prefix)
	if rest == "" {
		return nil, false
	}
	return strings.Split(rest, "/"), true
}

// _ is a type assertion
var _ unixfs.FSCursorOps = ((*DotGitFSCursorOps)(nil))
