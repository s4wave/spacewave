package unixfs_git

type dotGitNodeKind uint8

const (
	dotGitNodeKindDir dotGitNodeKind = iota
	dotGitNodeKindFile
)

type dotGitNode struct {
	name     string
	path     []string
	kind     dotGitNodeKind
	content  []byte
	children []*dotGitNode
}

func newDotGitRootNode() *dotGitNode {
	return &dotGitNode{
		kind: dotGitNodeKindDir,
		children: []*dotGitNode{
			newDotGitFileNode("HEAD", []string{"HEAD"}, nil),
			newDotGitFileNode("config", []string{"config"}, nil),
			newDotGitFileNode("description", []string{"description"}, nil),
			newDotGitDirNode("hooks", []string{"hooks"}),
			newDotGitDirNode("info", []string{"info"}),
			newDotGitDirNode("objects", []string{"objects"}),
			newDotGitDirNode("refs", []string{"refs"}),
		},
	}
}

func newDotGitDirNode(name string, path []string) *dotGitNode {
	return &dotGitNode{name: name, path: path, kind: dotGitNodeKindDir}
}

func newDotGitFileNode(name string, path []string, content []byte) *dotGitNode {
	return &dotGitNode{name: name, path: path, kind: dotGitNodeKindFile, content: content}
}

func (n *dotGitNode) child(name string) *dotGitNode {
	for _, child := range n.children {
		if child.name == name {
			return child
		}
	}
	return nil
}
