package bldr_manifest

import "errors"

var (
	// ErrNotFoundManifest is returned if the manifest was not found.
	ErrNotFoundManifest = errors.New("manifest not found")
	// ErrEmptyManifestID is returned if the manifest ID was empty.
	ErrEmptyManifestID = errors.New("manifest id cannot be empty")
	// ErrEmptyBuildType is returned if the build type was empty.
	ErrEmptyBuildType = errors.New("build type cannot be empty")
	// ErrEmptyPlatformID is returned if the platform ID was empty.
	ErrEmptyPlatformID = errors.New("platform id cannot be empty")
	// ErrEmptyEntrypoint is returned if the entrypoint was empty.
	ErrEmptyEntrypoint = errors.New("entrypoint cannot be empty")
	// ErrEmptyPath is returned if the path was empty.
	ErrEmptyPath = errors.New("path cannot be empty")
)
