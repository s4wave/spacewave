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

// GCGraph is an OPFS-backed GC reference graph.
type GCGraph struct {
	root       js.Value
	lockPrefix string
}

// NewGCGraph creates a GCGraph rooted at the given OPFS directory.
// The directory should be dedicated to the GC graph (e.g. <vol>/gc/graph/).
// lockPrefix is prepended to per-file WebLock names.
func NewGCGraph(root js.Value, lockPrefix string) (*GCGraph, error) {
	// Ensure subdirectories exist.
	for _, name := range []string{dirNodes, dirEdges, dirIncoming, dirRoots} {
		if _, err := opfs.GetDirectory(root, name, true); err != nil {
			return nil, errors.Wrap(err, "create "+name)
		}
	}
	return &GCGraph{root: root, lockPrefix: lockPrefix}, nil
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
	edgesDir, err := g.getSubSubDir(dirEdges, sh, true)
	if err != nil {
		return errors.Wrap(err, "edges subdir")
	}
	if err := g.writeFile(edgesDir, oh, content); err != nil {
		return errors.Wrap(err, "write forward edge")
	}

	// Reverse edge: incoming/<object-hash>/<subject-hash>
	inDir, err := g.getSubSubDir(dirIncoming, oh, true)
	if err != nil {
		return errors.Wrap(err, "incoming subdir")
	}
	return errors.Wrap(g.writeFile(inDir, sh, content), "write reverse edge")
}

// RemoveRef removes a single gc/ref edge from subject to object.
// Removing a non-existent edge is a no-op.
func (g *GCGraph) RemoveRef(ctx context.Context, subject, object string) error {
	sh := hashName(subject)
	oh := hashName(object)

	// Forward edge.
	edgesDir, err := g.getSubSubDir(dirEdges, sh, false)
	if err == nil {
		_ = g.deleteFile(edgesDir, oh)
	}

	// Reverse edge.
	inDir, err := g.getSubSubDir(dirIncoming, oh, false)
	if err == nil {
		_ = g.deleteFile(inDir, sh)
	}
	return nil
}

// ensureNode creates a node inventory entry if it does not exist.
func (g *GCGraph) ensureNode(iri string) error {
	nodesDir, err := opfs.GetDirectory(g.root, dirNodes, true)
	if err != nil {
		return err
	}
	h := hashName(iri)
	exists, err := opfs.FileExists(nodesDir, h)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return g.writeFile(nodesDir, h, []byte(iri))
}

// AddRoot adds a node to the root set.
func (g *GCGraph) AddRoot(ctx context.Context, iri string) error {
	rootsDir, err := opfs.GetDirectory(g.root, dirRoots, true)
	if err != nil {
		return errors.Wrap(err, "roots dir")
	}
	if err := g.ensureNode(iri); err != nil {
		return errors.Wrap(err, "ensure root node")
	}
	return g.writeFile(rootsDir, hashName(iri), []byte(iri))
}

// RemoveRoot removes a node from the root set.
func (g *GCGraph) RemoveRoot(ctx context.Context, iri string) error {
	rootsDir, err := opfs.GetDirectory(g.root, dirRoots, false)
	if err != nil {
		return nil
	}
	_ = g.deleteFile(rootsDir, hashName(iri))
	return nil
}

// RemoveNode removes a node from the node inventory.
func (g *GCGraph) RemoveNode(ctx context.Context, iri string) error {
	nodesDir, err := opfs.GetDirectory(g.root, dirNodes, false)
	if err != nil {
		return nil
	}
	_ = g.deleteFile(nodesDir, hashName(iri))
	return nil
}

// getSubSubDir gets or creates a nested subdirectory (e.g. edges/<hash>/).
func (g *GCGraph) getSubSubDir(parent, child string, create bool) (js.Value, error) {
	pDir, err := opfs.GetDirectory(g.root, parent, create)
	if err != nil {
		return js.Undefined(), err
	}
	return opfs.GetDirectory(pDir, child, create)
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
var _ block_gc.RefGraphOps = (*GCGraph)(nil)
