package space_unixfs

import space_unixfs_projectedpath "github.com/s4wave/spacewave/core/space/unixfs/projectedpath"

// ProjectedPath is a parsed projected filesystem path.
type ProjectedPath = space_unixfs_projectedpath.ProjectedPath

// ParseProjectedPath parses u/{idx}/so/{soId}... into projected path metadata.
func ParseProjectedPath(path string) (*ProjectedPath, error) {
	return space_unixfs_projectedpath.Parse(path)
}
