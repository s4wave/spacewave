package provider_spacewave

import "context"

// SyncReplaceData uploads a replacement pack through the authenticated write-ticket
// path. The catalog atomically supersedes replacedPackIDs and schedules their
// physical deletion. The caller must first make every still-reachable block
// available outside those packs. Each request accepts at most 32 replaced IDs.
// bodyHash is the SHA-256 digest of packData. A replacement conflict is returned
// without changing the replacement set or retrying against a different catalog.
func (c *SessionClient) SyncReplaceData(
	ctx context.Context,
	resourceID, packID string,
	blockCount int,
	packData, bodyHash, bloomFilter []byte,
	bloomFormatVersion uint32,
	replacedPackIDs []string,
) error {
	return c.syncPushDataWithProgress(ctx, resourceID, &syncPushPack{
		packID:             packID,
		blockCount:         blockCount,
		bodyHash:           bodyHash,
		bloomFilter:        bloomFilter,
		bloomFormatVersion: bloomFormatVersion,
		replacedPackIDs:    replacedPackIDs,
	}, packData, nil)
}
