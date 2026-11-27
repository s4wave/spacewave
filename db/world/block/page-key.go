package world_block

// PageKey identifies a count of pages: their tree and level.
type PageKey struct {
	// Tree is the part of the World the pages belong to.
	Tree PageTree
	// Level is the pages' level, as Page defines it.
	Level int
}
