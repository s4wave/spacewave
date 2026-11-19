package world

// GraphPathQueryResult contains graph path traversal results.
type GraphPathQueryResult struct {
	// ObjectKeys is the set of reached object keys.
	ObjectKeys []string
	// Quads is the set of traversed quads when requested.
	Quads []GraphQuad
}
