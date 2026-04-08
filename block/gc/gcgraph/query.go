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
	dir, err := g.getSubSubDir(dirEdges, h, false)
	if err != nil {
		if opfs.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "edges subdir")
	}

	names, err := opfs.ListDirectory(dir)
	if err != nil {
		return nil, errors.Wrap(err, "list edges")
	}

	targets := make([]string, 0, len(names))
	for _, name := range names {
		data, err := g.readFileContent(dir, name)
		if err != nil {
			continue
		}
		_, obj, ok := parseEdgeContent(data)
		if ok {
			targets = append(targets, obj)
		}
	}
	return targets, nil
}

// GetIncomingRefs returns all sources with gc/ref edges pointing to the given node.
func (g *GCGraph) GetIncomingRefs(ctx context.Context, node string) ([]string, error) {
	h := hashName(node)
	dir, err := g.getSubSubDir(dirIncoming, h, false)
	if err != nil {
		if opfs.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "incoming subdir")
	}

	names, err := opfs.ListDirectory(dir)
	if err != nil {
		return nil, errors.Wrap(err, "list incoming")
	}

	sources := make([]string, 0, len(names))
	for _, name := range names {
		data, err := g.readFileContent(dir, name)
		if err != nil {
			continue
		}
		subj, _, ok := parseEdgeContent(data)
		if ok {
			sources = append(sources, subj)
		}
	}
	return sources, nil
}

// HasIncomingRefs checks if a node has any incoming gc/ref edges.
// Excludes edges from "unreferenced".
func (g *GCGraph) HasIncomingRefs(ctx context.Context, node string) (bool, error) {
	h := hashName(node)
	dir, err := g.getSubSubDir(dirIncoming, h, false)
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

	for _, name := range names {
		data, err := g.readFileContent(dir, name)
		if err != nil {
			continue
		}
		subj, _, ok := parseEdgeContent(data)
		if ok && subj != block_gc.NodeUnreferenced {
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
	nodesDir, err := opfs.GetDirectory(g.root, dirNodes, false)
	if err != nil {
		if opfs.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "nodes dir")
	}
	return g.readInventory(nodesDir)
}

// GetRootNodes returns all node IRIs in the root set.
func (g *GCGraph) GetRootNodes(ctx context.Context) ([]string, error) {
	rootsDir, err := opfs.GetDirectory(g.root, dirRoots, false)
	if err != nil {
		if opfs.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "roots dir")
	}
	return g.readInventory(rootsDir)
}

// readInventory lists a flat directory and reads IRI content from each file.
func (g *GCGraph) readInventory(dir js.Value) ([]string, error) {
	names, err := opfs.ListDirectory(dir)
	if err != nil {
		return nil, err
	}
	iris := make([]string, 0, len(names))
	for _, name := range names {
		data, err := g.readFileContent(dir, name)
		if err != nil {
			continue
		}
		if len(data) > 0 {
			iris = append(iris, string(data))
		}
	}
	return iris, nil
}
