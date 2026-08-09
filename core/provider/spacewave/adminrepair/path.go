package adminrepair

import (
	"path"
)

// Path returns the cloud API path for pack metadata repair.
func Path(resourceID string) string {
	return path.Join("/api/admin/bstore", resourceID, "pack-metadata-repair")
}
