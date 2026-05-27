package downloadurl

import (
	"strings"

	space_unixfs_projectedpath "github.com/s4wave/spacewave/core/space/unixfs/projectedpath"
)

const fsPathPrefix = "/fs/"

// Request is the parsed Space download URL request.
type Request struct {
	// SessionIdx is the projected session index.
	SessionIdx uint32
	// SharedObjectID is the projected shared object identifier.
	SharedObjectID string
	// ProjectedPath is the normalized projected path.
	ProjectedPath string
}

// Parse parses /fs/u/{idx}/so/{soId}/... download URLs.
func Parse(path string) (*Request, error) {
	rest := strings.TrimPrefix(path, fsPathPrefix)
	projected, err := space_unixfs_projectedpath.Parse(rest)
	if err != nil {
		return nil, err
	}

	return &Request{
		SessionIdx:     projected.SessionIdx,
		SharedObjectID: projected.SharedObjectID,
		ProjectedPath:  projected.Path,
	}, nil
}
