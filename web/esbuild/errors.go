package bldr_esbuild

import (
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
)

// BuildResultToErr converts a BuildResult into a single error, if any.
func BuildResultToErr(res esbuild.BuildResult) error {
	if len(res.Errors) == 0 {
		return nil
	}
	return errors.New(res.Errors[0].Text)
}

// ResolveResultToErr converts a BuildResult into a single error, if any.
func ResolveResultToErr(res esbuild.ResolveResult) error {
	if len(res.Errors) == 0 {
		return nil
	}
	return errors.New(res.Errors[0].Text)
}
