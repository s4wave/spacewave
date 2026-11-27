package s4wave_world

// KnownErrorCode is the highest WorldErrorCode this build decodes. Each
// request that can return a refusal specifies it as its known_error_code.
const KnownErrorCode = WorldErrorCode_WORLD_ERROR_CODE_DURABILITY_UNKNOWN

// Decodes reports whether a client whose request specifies known as its
// known_error_code decodes code. Every build decodes the codes through
// OBJECT_NOT_FOUND. A client built before known_error_code existed specifies
// UNSPECIFIED.
func Decodes(known, code WorldErrorCode) bool {
	return code <= WorldErrorCode_WORLD_ERROR_CODE_OBJECT_NOT_FOUND || code <= known
}
