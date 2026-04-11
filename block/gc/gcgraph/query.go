//go:build js

package gcgraph

import (
	"context"
	"syscall/js"

	block_gc "github.com/aperturerobotics/hydra/block/gc"
	"github.com/aperturerobotics/hydra/opfs"
	"github.com/pkg/errors"
)

// GetOutgoingRefs returns all targets of gc/ref edges from the given node.
func (g *GCGraph) GetOutgoingRefs(ctx context.Context, node string) ([]string, error) {
	h := hashName(node)
	dir, err := opfs.GetDirectory(g.edgesDir, h, false)
	if err != nil {
		if opfs.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "edges subdir")
	}
	return g.readEdgeTargets(dir)
}

// GetIncomingRefs returns all sources with gc/ref edges pointing to the given node.
func (g *GCGraph) GetIncomingRefs(ctx context.Context, node string) ([]string, error) {
	h := hashName(node)
	dir, err := opfs.GetDirectory(g.incomingDir, h, false)
	if err != nil {
		if opfs.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "incoming subdir")
	}
	return g.readEdgeSources(dir)
}

// HasIncomingRefs checks if a node has any incoming gc/ref edges
// besides those from "unreferenced". Uses the pre-computed hash of
// NodeUnreferenced for filename comparison to avoid file I/O.
func (g *GCGraph) HasIncomingRefs(ctx context.Context, node string) (bool, error) {
	return g.HasIncomingRefsExcluding(ctx, node)
}

// HasIncomingRefsExcluding checks if a node has any incoming gc/ref edges
// besides those from "unreferenced" and the specified source nodes.
func (g *GCGraph) HasIncomingRefsExcluding(
	ctx context.Context,
	node string,
	excluded ...string,
) (bool, error) {
	h := hashName(node)
	dir, err := opfs.GetDirectory(g.incomingDir, h, false)
	if err != nil {
		if opfs.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrap(err, "incoming subdir")
	}

	names, err := opfs.ListDirectory(dir)
	if err != nil {
		return false, errors.Wrap(err, "list incoming")
	}

	excludedHashes := make(map[string]struct{}, len(excluded)+1)
	excludedHashes[unreferencedHash] = struct{}{}
	for _, src := range excluded {
		excludedHashes[hashName(src)] = struct{}{}
	}
	for _, name := range names {
		if _, ok := excludedHashes[name]; !ok {
			return true, nil
		}
	}
	return false, nil
}

// GetUnreferencedNodes returns all nodes linked from "unreferenced".
func (g *GCGraph) GetUnreferencedNodes(ctx context.Context) ([]string, error) {
	return g.GetOutgoingRefs(ctx, block_gc.NodeUnreferenced)
}

// IterateNodes returns all node IRIs in the node inventory.
func (g *GCGraph) IterateNodes(ctx context.Context) ([]string, error) {
	return g.readInventory(g.nodesDir)
}

// GetRootNodes returns all node IRIs in the root set.
func (g *GCGraph) GetRootNodes(ctx context.Context) ([]string, error) {
	return g.readInventory(g.rootsDir)
}

// readEdgeTargets lists edge files and extracts the object (target) IRI.
// Skips files that cannot be read (concurrent deletion).
func (g *GCGraph) readEdgeTargets(dir js.Value) ([]string, error) {
	names, err := opfs.ListDirectory(dir)
	if err != nil {
		return nil, errors.Wrap(err, "list edges")
	}
	targets := make([]string, 0, len(names))
	for _, name := range names {
		data, err := g.readFileContent(dir, name)
		if err != nil {
			if opfs.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		if _, obj, ok := parseEdgeContent(data); ok {
			targets = append(targets, obj)
		}
	}
	return targets, nil
}

// readEdgeSources lists edge files and extracts the subject (source) IRI.
// Skips files that cannot be read (concurrent deletion).
func (g *GCGraph) readEdgeSources(dir js.Value) ([]string, error) {
	names, err := opfs.ListDirectory(dir)
	if err != nil {
		return nil, errors.Wrap(err, "list incoming")
	}
	sources := make([]string, 0, len(names))
	for _, name := range names {
		data, err := g.readFileContent(dir, name)
		if err != nil {
			if opfs.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		if subj, _, ok := parseEdgeContent(data); ok {
			sources = append(sources, subj)
		}
	}
	return sources, nil
}

// readInventory lists a flat directory and reads IRI content from each file.
// Skips files that cannot be read (concurrent deletion).
func (g *GCGraph) readInventory(dir js.Value) ([]string, error) {
	names, err := opfs.ListDirectory(dir)
	if err != nil {
		return nil, err
	}
	iris := make([]string, 0, len(names))
	for _, name := range names {
		data, err := g.readFileContent(dir, name)
		if err != nil {
			if opfs.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		if len(data) > 0 {
			iris = append(iris, string(data))
		}
	}
	return iris, nil
}
