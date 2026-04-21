// Package unixfs_tar implements a read-only FSCursor backed by a tar archive.
package unixfs_tar

import (
	"archive/tar"
	"bytes"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

// tarNode is a node in the in-memory tar directory tree.
type tarNode struct {
	name     string
	mode     fs.FileMode
	modTime  time.Time
	size     int64
	isDir    bool
	isLink   bool
	ra       io.ReaderAt
	offset   int64
	linkTgt  string
	children []*tarNode
	childMap map[string]*tarNode
}

// addChild adds a child node, replacing any existing child with the same name.
func (n *tarNode) addChild(child *tarNode) {
	if n.childMap == nil {
		n.childMap = make(map[string]*tarNode)
	}
	if existing, ok := n.childMap[child.name]; ok {
		for i, c := range n.children {
			if c == existing {
				n.children[i] = child
				break
			}
		}
		n.childMap[child.name] = child
		return
	}
	n.children = append(n.children, child)
	n.childMap[child.name] = child
}

// sortChildren sorts children by name.
func (n *tarNode) sortChildren() {
	sort.Slice(n.children, func(i, j int) bool {
		return n.children[i].name < n.children[j].name
	})
}

// readCounter wraps an io.Reader and tracks bytes read.
type readCounter struct {
	r   io.Reader
	pos int64
}

// Read reads from the underlying reader and tracks position.
func (rc *readCounter) Read(p []byte) (int, error) {
	n, err := rc.r.Read(p)
	rc.pos += int64(n)
	return n, err
}

// parseTar parses a tar archive from an io.ReaderAt and builds a tarNode tree.
// The returned root node is a directory containing all entries.
func parseTar(ra io.ReaderAt, size int64) (*tarNode, error) {
	root := &tarNode{
		isDir:    true,
		mode:     fs.ModeDir | 0o755,
		childMap: make(map[string]*tarNode),
	}
	nodes := map[string]*tarNode{".": root}

	counter := &readCounter{r: io.NewSectionReader(ra, 0, size)}
	tr := tar.NewReader(counter)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if hdr.Typeflag == tar.TypeXGlobalHeader {
			continue
		}

		name := path.Clean(hdr.Name)
		if name == "." {
			continue
		}

		// strip trailing slash from dirs
		name = strings.TrimSuffix(name, "/")

		switch hdr.Typeflag {
		case tar.TypeDir:
			node := &tarNode{
				name:    path.Base(name),
				isDir:   true,
				mode:    hdr.FileInfo().Mode(),
				modTime: hdr.ModTime,
			}
			ensureParent(nodes, root, name)
			parent := nodes[path.Dir(name)]
			parent.addChild(node)
			if nodes[name] != nil {
				// implicit dir already exists, merge children
				node.children = nodes[name].children
				node.childMap = nodes[name].childMap
			}
			nodes[name] = node

		case tar.TypeReg:
			dataOffset := counter.pos
			node := &tarNode{
				name:    path.Base(name),
				mode:    hdr.FileInfo().Mode(),
				modTime: hdr.ModTime,
				size:    hdr.Size,
				ra:      ra,
				offset:  dataOffset,
			}
			ensureParent(nodes, root, name)
			parent := nodes[path.Dir(name)]
			parent.addChild(node)
			nodes[name] = node

		case tar.TypeSymlink:
			node := &tarNode{
				name:    path.Base(name),
				isLink:  true,
				mode:    fs.ModeSymlink | 0o777,
				modTime: hdr.ModTime,
				linkTgt: hdr.Linkname,
			}
			ensureParent(nodes, root, name)
			parent := nodes[path.Dir(name)]
			parent.addChild(node)
			nodes[name] = node

		case tar.TypeLink:
			// hardlink: resolve to target node's data
			tgt := path.Clean(hdr.Linkname)
			if tgtNode, ok := nodes[tgt]; ok && !tgtNode.isDir && !tgtNode.isLink {
				node := &tarNode{
					name:    path.Base(name),
					mode:    tgtNode.mode,
					modTime: tgtNode.modTime,
					size:    tgtNode.size,
					ra:      tgtNode.ra,
					offset:  tgtNode.offset,
				}
				ensureParent(nodes, root, name)
				parent := nodes[path.Dir(name)]
				parent.addChild(node)
				nodes[name] = node
			}
			// if target not found, skip silently

		default:
			// skip other types (block devices, char devices, fifos, etc.)
		}
	}

	// sort all directory children
	sortAll(root)

	return root, nil
}

// ensureParent ensures all parent directories of name exist in the tree.
func ensureParent(nodes map[string]*tarNode, root *tarNode, name string) {
	dir := path.Dir(name)
	if dir == "." {
		return
	}
	if _, ok := nodes[dir]; ok {
		return
	}

	// recursively ensure grandparent
	ensureParent(nodes, root, dir)

	node := &tarNode{
		name:     path.Base(dir),
		isDir:    true,
		mode:     fs.ModeDir | 0o755,
		childMap: make(map[string]*tarNode),
	}
	parent := nodes[path.Dir(dir)]
	parent.addChild(node)
	nodes[dir] = node
}

// sortAll recursively sorts children of all directories.
func sortAll(node *tarNode) {
	if !node.isDir {
		return
	}
	node.sortChildren()
	for _, child := range node.children {
		sortAll(child)
	}
}

// parseTarFromReader reads the entire tar into memory and parses it.
func parseTarFromReader(r io.Reader) (*tarNode, io.ReaderAt, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	ra := bytes.NewReader(data)
	root, err := parseTar(ra, int64(len(data)))
	if err != nil {
		return nil, nil, err
	}
	return root, ra, nil
}
