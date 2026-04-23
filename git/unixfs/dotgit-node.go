package unixfs_git

import "github.com/go-git/go-git/v6/plumbing"

type dotGitNodeKind uint8

const (
	dotGitNodeKindDir dotGitNodeKind = iota
	dotGitNodeKindFile
)

type dotGitNode struct {
	name     string
	path     []string
	kind     dotGitNodeKind
	hash     plumbing.Hash
	content  []byte
	children []*dotGitNode
}

func newDotGitRootNode() *dotGitNode {
	objects := newDotGitDirNode("objects", []string{"objects"})
	objects.children = []*dotGitNode{
		newDotGitDirNode("info", []string{"objects", "info"}),
		newDotGitDirNode("pack", []string{"objects", "pack"}),
	}
	objects.children[0].children = []*dotGitNode{
		newDotGitFileNode("packs", []string{"objects", "info", "packs"}, nil),
	}
	return &dotGitNode{
		kind: dotGitNodeKindDir,
		children: []*dotGitNode{
			newDotGitFileNode("HEAD", []string{"HEAD"}, nil),
			newDotGitFileNode("config", []string{"config"}, nil),
			newDotGitFileNode("description", []string{"description"}, nil),
			newDotGitDirNode("hooks", []string{"hooks"}),
			newDotGitDirNode("info", []string{"info"}),
			newDotGitDirNode("logs", []string{"logs"}),
			newDotGitDirNode("modules", []string{"modules"}),
			objects,
			newDotGitFileNode("packed-refs", []string{"packed-refs"}, nil),
			newDotGitDirNode("refs", []string{"refs"}),
			newDotGitFileNode("shallow", []string{"shallow"}, nil),
		},
	}
}

func newDotGitDirNode(name string, path []string) *dotGitNode {
	return &dotGitNode{name: name, path: path, kind: dotGitNodeKindDir}
}

func newDotGitFileNode(name string, path []string, content []byte) *dotGitNode {
	return &dotGitNode{name: name, path: path, kind: dotGitNodeKindFile, content: content}
}

func newDotGitObjectFileNode(hash plumbing.Hash) *dotGitNode {
	name := hash.String()[2:]
	return &dotGitNode{
		name: name,
		path: []string{"objects", hash.String()[:2], name},
		kind: dotGitNodeKindFile,
		hash: hash,
	}
}

func (n *dotGitNode) child(name string) *dotGitNode {
	for _, child := range n.children {
		if child.name == name {
			return child
		}
	}
	return nil
}
