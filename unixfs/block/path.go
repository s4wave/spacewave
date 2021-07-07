package unixfs_block

// NewFSPath builds a new filesystem path.
func NewFSPath(path []string) *FSPath {
	return &FSPath{
		Nodes: path,
	}
}

// Validate validates the path.
func (p *FSPath) Validate() error {
	if len(p.GetNodes()) == 0 {
		return ErrEmptyPath
	}
	for _, dir := range p.GetNodes() {
		if err := ValidateDirectoryName(dir); err != nil {
			return err
		}
	}
	return nil
}

// PathsToStringSlices converts a set of paths to a list of string slices.
func PathsToStringSlices(paths ...*FSPath) [][]string {
	out := make([][]string, len(paths))
	for i, x := range paths {
		out[i] = x.GetNodes()
	}
	return out
}

// StringSlicesToPaths converts the string slices to paths.
func StringSlicesToPaths(paths [][]string) []*FSPath {
	out := make([]*FSPath, len(paths))
	for i, p := range paths {
		out[i] = NewFSPath(p)
	}
	return out
}
