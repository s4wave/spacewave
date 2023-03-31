package bldr_project

import "errors"

var (
	// ErrEmptyProjectID is returned if the project ID was empty.
	ErrEmptyProjectID = errors.New("project id cannot be empty")
	// ErrEmptyRemoteID is returned if the remote ID was empty.
	ErrEmptyRemoteID = errors.New("remote id cannot be empty")
	// ErrRemoteNotFound is returned if the remote ID was not found.
	ErrRemoteNotFound = errors.New("remote not found")
	// ErrManifestConfNotFound is returned if the manifest config was not found.
	ErrManifestConfNotFound = errors.New("manifest config not found")
)
