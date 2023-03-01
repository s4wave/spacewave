package bldr_project

import "errors"

var (
	// ErrEmptyProjectID is returned if the project ID was empty.
	ErrEmptyProjectID = errors.New("project id cannot be empty")
)
