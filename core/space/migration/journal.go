package space_migration

import "github.com/pkg/errors"

// NewMigrationJournal snapshots a complete immutable preview for durable
// migration progress. Callers may persist the returned protobuf bytes; the
// journal owns a deep clone and never aliases the planner's preview.
func NewMigrationJournal(operationID string, preview *MigrationPreview, retentionUntilUnixSeconds uint64) (*MigrationJournal, error) {
	if operationID == "" {
		return nil, errors.New("migration journal operation ID is required")
	}
	if preview == nil {
		return nil, errors.New("migration journal preview is required")
	}
	if preview.Digest == "" || preview.Digest != digestPreview(preview) {
		return nil, errors.New("migration journal preview digest is invalid")
	}
	mappings := make([]*MigrationIdentityMapping, 0, len(preview.IdentityMappings))
	for _, mapping := range preview.IdentityMappings {
		if mapping != nil {
			mappings = append(mappings, mapping.CloneVT())
		}
	}
	sourceBlockStoreID, destinationBlockStoreID := "", ""
	for _, mapping := range mappings {
		if mapping.Kind == MigrationReferenceKind_MIGRATION_REFERENCE_KIND_BLOCK_STORE {
			sourceBlockStoreID, destinationBlockStoreID = mapping.Source, mapping.Destination
			break
		}
	}
	return &MigrationJournal{
		OperationId:               operationID,
		PreviewDigest:             preview.Digest,
		SourceSpaceId:             preview.SourceSpaceId,
		DestinationSpaceId:        preview.DestinationSpaceId,
		Progress:                  preview.Progress.CloneVT(),
		Terminal:                  preview.Result.CloneVT(),
		SourceRevision:            preview.SourceRevision,
		DestinationRevision:       preview.DestinationRevision,
		SourceBlockStoreId:        sourceBlockStoreID,
		DestinationBlockStoreId:   destinationBlockStoreID,
		IdentityMappings:          mappings,
		Preview:                   preview.CloneVT(),
		RetentionUntilUnixSeconds: retentionUntilUnixSeconds,
	}, nil
}
