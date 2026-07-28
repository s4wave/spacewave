package artifact

// Generation identifies one immutable published release artifact.
type Generation struct {
	ID           string
	ReleaseDir   string
	PrerenderDir string
}
