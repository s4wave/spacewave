package space_migration

// Classification describes the migration safety policy for an ObjectType.
type Classification int32

const (
	// ClassificationUnclassified is a typed blocker when no handler is present.
	ClassificationUnclassified Classification = Classification(MigrationClassification_MIGRATION_CLASSIFICATION_UNCLASSIFIED)
	// ClassificationSpaceLocalOpaque allows cloning without reference rewriting.
	ClassificationSpaceLocalOpaque Classification = Classification(MigrationClassification_MIGRATION_CLASSIFICATION_SPACE_LOCAL_OPAQUE)
	// ClassificationRewrite requires typed reference rewriting.
	ClassificationRewrite Classification = Classification(MigrationClassification_MIGRATION_CLASSIFICATION_REWRITE)
	// ClassificationExternalRef preserves and discloses an external reference.
	ClassificationExternalRef Classification = Classification(MigrationClassification_MIGRATION_CLASSIFICATION_EXTERNAL_REF)
	// ClassificationNonMigratable refuses the object unless it is excluded.
	ClassificationNonMigratable Classification = Classification(MigrationClassification_MIGRATION_CLASSIFICATION_NON_MIGRATABLE)
)

// Proto returns the wire classification value.
func (c Classification) Proto() MigrationClassification {
	return MigrationClassification(c)
}
func validClassification(c Classification) bool {
	switch c {
	case ClassificationUnclassified, ClassificationSpaceLocalOpaque, ClassificationRewrite, ClassificationExternalRef, ClassificationNonMigratable:
		return true
	default:
		return false
	}
}
