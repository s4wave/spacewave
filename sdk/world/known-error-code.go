package s4wave_world

// KnownErrorCode is the highest WorldErrorCode this build decodes. Each
// request that can return a refusal names it as its known_error_code.
const KnownErrorCode = WorldErrorCode_WORLD_ERROR_CODE_REPLAY_WAITS

// Decodes reports whether a client whose request names known as its
// known_error_code decodes code. Every build decodes the codes through
// OBJECT_NOT_FOUND; a client built before known_error_code existed names
// UNSPECIFIED.
func Decodes(known, code WorldErrorCode) bool {
	return code <= WorldErrorCode_WORLD_ERROR_CODE_OBJECT_NOT_FOUND || code <= known
}
