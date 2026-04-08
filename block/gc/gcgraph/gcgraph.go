//go:build js

// Package gcgraph implements the GC reference graph on OPFS.
// Each edge, node inventory entry, and root-set entry is an individual
// file under a structured directory layout. Per-file locking provides
// concurrency safety.
package gcgraph

import (
	"context"
	"encoding/hex"
	"strings"
	"syscall/js"

	block_gc "github.com/aperturerobotics/hydra/block/gc"
	"github.com/aperturerobotics/hydra/opfs"
	"github.com/aperturerobotics/hydra/opfs/filelock"
	"github.com/pkg/errors"
	"github.com/zeebo/blake3"
)

// Directory names within the graph root.
const (
	dirNodes    = "nodes"
	dirEdges    = "edges"
	dirIncoming = "incoming"
	dirRoots    = "roots"
)

// hashName produces a short hex filename from an IRI string.
func hashName(iri string) string {
	h := blake3.Sum256([]byte(iri))
	return hex.EncodeToString(h[:16])
}

// unreferencedHash is the pre-computed hash of NodeUnreferenced for
// fast filename comparison in HasIncomingRefs.
var unreferencedHash = hashName(block_gc.NodeUnreferenced)

// GCGraph is an OPFS-backed GC reference graph.
type GCGraph struct {
	root       js.Value
	lockPrefix string
	// Cached directory handles to avoid repeated GetDirectory calls.
	nodesDir    js.Value
	edgesDir    js.Value
	incomingDir js.Value
	rootsDir    js.Value
}

// NewGCGraph creates a GCGraph rooted at the given OPFS directory.
// lockPrefix is prepended to per-file WebLock names.
func NewGCGraph(root js.Value, lockPrefix string) (*GCGraph, error) {
	g := &GCGraph{root: root, lockPrefix: lockPrefix}
	var err error
	g.nodesDir, err = opfs.GetDirectory(root, dirNodes, true)
	if err != nil {
		return nil, errors.Wrap(err, "create "+dirNodes)
	}
	g.edgesDir, err = opfs.GetDirectory(root, dirEdges, true)
	if err != nil {
		return nil, errors.Wrap(err, "create "+dirEdges)
	}
	g.incomingDir, err = opfs.GetDirectory(root, dirIncoming, true)
	if err != nil {
		return nil, errors.Wrap(err, "create "+dirIncoming)
	}
	g.rootsDir, err = opfs.GetDirectory(root, dirRoots, true)
	if err != nil {
		return nil, errors.Wrap(err, "create "+dirRoots)
	}
	return g, nil
}

// AddRef adds a gc/ref edge from subject to object. Idempotent.
// Ensures node inventory entries exist for both endpoints.
func (g *GCGraph) AddRef(ctx context.Context, subject, object string) error {
	if err := g.ensureNode(subject); err != nil {
		return errors.Wrap(err, "ensure subject node")
	}
	if err := g.ensureNode(object); err != nil {
		return errors.Wrap(err, "ensure object node")
	}

	sh := hashName(subject)
	oh := hashName(object)
	content := []byte(subject + "\n" + object)

	// Forward edge: edges/<subject-hash>/<object-hash>
	edgeSubDir, err := opfs.GetDirectory(g.edgesDir, sh, true)
	if err != nil {
		return errors.Wrap(err, "edges subdir")
	}
	if err := g.writeFile(edgeSubDir, oh, content); err != nil {
		return errors.Wrap(err, "write forward edge")
	}

	// Reverse edge: incoming/<object-hash>/<subject-hash>
	inSubDir, err := opfs.GetDirectory(g.incomingDir, oh, true)
	if err != nil {
		return errors.Wrap(err, "incoming subdir")
	}
	return errors.Wrap(g.writeFile(inSubDir, sh, content), "write reverse edge")
}

// RemoveRef removes a single gc/ref edge from subject to object.
// Removing a non-existent edge is a no-op.
func (g *GCGraph) RemoveRef(ctx context.Context, subject, object string) error {
	sh := hashName(subject)
	oh := hashName(object)

	// Forward edge.
	edgeSubDir, err := opfs.GetDirectory(g.edgesDir, sh, false)
	if err == nil {
		_ = g.deleteFile(edgeSubDir, oh)
	}

	// Reverse edge.
	inSubDir, err := opfs.GetDirectory(g.incomingDir, oh, false)
	if err == nil {
		_ = g.deleteFile(inSubDir, sh)
	}
	return nil
}

// ensureNode creates a node inventory entry if it does not exist.
func (g *GCGraph) ensureNode(iri string) error {
	h := hashName(iri)
	exists, err := opfs.FileExists(g.nodesDir, h)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return g.writeFile(g.nodesDir, h, []byte(iri))
}

// AddRoot adds a node to the root set.
func (g *GCGraph) AddRoot(ctx context.Context, iri string) error {
	if err := g.ensureNode(iri); err != nil {
		return errors.Wrap(err, "ensure root node")
	}
	return g.writeFile(g.rootsDir, hashName(iri), []byte(iri))
}

// RemoveRoot removes a node from the root set.
func (g *GCGraph) RemoveRoot(ctx context.Context, iri string) error {
	return g.deleteFile(g.rootsDir, hashName(iri))
}

// RemoveNode removes a node from the node inventory.
func (g *GCGraph) RemoveNode(ctx context.Context, iri string) error {
	return g.deleteFile(g.nodesDir, hashName(iri))
}

// writeFile writes content to a file using per-file locking.
func (g *GCGraph) writeFile(dir js.Value, name string, content []byte) error {
	f, release, err := filelock.AcquireFile(dir, name, g.lockPrefix, true)
	if err != nil {
		return err
	}
	defer release()

	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.WriteAt(content, 0); err != nil {
		return err
	}
	return f.Flush()
}

// deleteFile removes a file, ignoring not-found errors.
func (g *GCGraph) deleteFile(dir js.Value, name string) error {
	err := opfs.DeleteFile(dir, name)
	if err != nil && opfs.IsNotFound(err) {
		return nil
	}
	return err
}

// readFileContent reads the full content of a file using per-file locking.
func (g *GCGraph) readFileContent(dir js.Value, name string) ([]byte, error) {
	f, release, err := filelock.AcquireFile(dir, name, g.lockPrefix, false)
	if err != nil {
		return nil, err
	}
	defer release()

	size, err := f.Size()
	if err != nil {
		return nil, err
	}
	buf := make([]byte, size)
	n, err := f.ReadAt(buf, 0)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// parseEdgeContent splits "subject\nobject" file content.
func parseEdgeContent(data []byte) (subject, object string, ok bool) {
	subject, object, ok = strings.Cut(string(data), "\n")
	return subject, object, ok
}

// _ is a type assertion
var _ block_gc.CollectorGraph = (*GCGraph)(nil)
