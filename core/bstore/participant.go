package bstore

// ValidateBlockStoreParticipantRole ensures the enum value is within the expected set.
// If allowUnknown is true, it will allow BlockStoreParticipantRole_UNKNOWN as a valid role.
func ValidateBlockStoreParticipantRole(role BlockStoreParticipantRole, allowUnknown bool) error {
	switch role {
	case BlockStoreParticipantRole_BlockStoreParticipantRole_READER,
		BlockStoreParticipantRole_BlockStoreParticipantRole_WRITER,
		BlockStoreParticipantRole_BlockStoreParticipantRole_OWNER:
		return nil
	case BlockStoreParticipantRole_BlockStoreParticipantRole_UNKNOWN:
		if allowUnknown {
			return nil
		}
		fallthrough
	default:
		return ErrInvalidBlockStoreParticipantRole
	}
}
