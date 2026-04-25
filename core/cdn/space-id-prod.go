//go:build !build_type_dev

package cdn

// SpaceID returns the well-known mounted public CDN Space ULID. Prod builds
// always return the hardcoded =ProvisionedSpaceID= for the public destination
// Space; the env-var override ships only with dev builds (see
// =space-id-dev.go=).
func SpaceID() string {
	return ProvisionedSpaceID
}
