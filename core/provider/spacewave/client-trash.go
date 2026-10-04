package provider_spacewave

import (
	"context"
	"path"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile"
)

// SyncTrash moves current packs to the trash and retires trash packs older
// than packfile.TrashAge from a resource-scoped block store catalog. Trash
// packs stay readable but no longer count for upload dedup. A retired pack
// leaves the catalog and is deleted after the retirement grace. The caller
// pulls to observe the change. A pack in the wrong state fails the request
// with a pack replacement conflict and changes nothing.
func (c *SessionClient) SyncTrash(ctx context.Context, resourceID string, req *packfile.TrashRequest) error {
	body, err := req.MarshalVT()
	if err != nil {
		return err
	}
	_, err = c.doPostBinary(ctx, path.Join("/api/bstore", resourceID, "sync/trash"), body, nil, SeedReasonMutation)
	return errors.Wrap(err, "sync trash")
}
