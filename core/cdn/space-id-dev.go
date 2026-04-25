//go:build build_type_dev

package cdn

import "os"

// SpaceID returns the well-known mounted public CDN Space ULID. In dev builds,
// =SPACEWAVE_CDN_SPACE_ID= may override it so the client can target the
// staging public CDN Space =01kpfs6hyxeamz1a5hwwqph291= (or any other test
// public Space) without rebuilding. This override is for public destination
// Spaces only, never private authoring Spaces.
func SpaceID() string {
	if env := os.Getenv("SPACEWAVE_CDN_SPACE_ID"); env != "" {
		return env
	}
	return ProvisionedSpaceID
}
