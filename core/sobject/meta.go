package sobject

const (
	// MaxBodyTypeLength is the maximum length of a body_type string.
	MaxBodyTypeLength = 1024
	// MaxBodyMetaLength is the maximum length of body_meta bytes.
	MaxBodyMetaLength = 4096
)

// isValidBodyTypeChars returns true if the string contains only valid characters.
func isValidBodyTypeChars(s string) bool {
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

// Validate validates the metadata.
func (m *SharedObjectMeta) Validate() error {
	bodyType := m.GetBodyType()
	if bodyType == "" || len(bodyType) > MaxBodyTypeLength || !isValidBodyTypeChars(bodyType) || len(m.GetBodyMeta()) > MaxBodyMetaLength {
		return ErrInvalidMeta
	}
	return nil
}
