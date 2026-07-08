package cdn

// DefaultBaseURL is the production Spacewave CDN origin. Builds honor
// SPACEWAVE_CDN_BASE_URL when the runtime explicitly supplies a staging or
// local mirror; unset clients use this production origin.
const DefaultBaseURL = "https://cdn.spacewave.app"
