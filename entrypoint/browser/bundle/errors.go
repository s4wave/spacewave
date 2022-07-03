package entrypoint_browser_bundle

import (
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
)

func EsbuildErrorsToError(res esbuild.BuildResult) error {
	if len(res.Errors) == 0 {
		return nil
	}
	return errors.New(res.Errors[0].Text)
}
