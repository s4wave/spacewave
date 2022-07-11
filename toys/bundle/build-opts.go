package main

import (
	browser "github.com/aperturerobotics/bldr/entrypoint/browser/bundle"
	esbuild "github.com/evanw/esbuild/pkg/api"
)

func defaultBanner() map[string]string {
	return map[string]string{
		"js": "// github.com/aperturerobotics/bldr/toys/bundle",
	}
}

// BundleComponentBuildOpts creates the BuildOpts for bundling a component.
//
// The repo root is used for tsconfig.
// Component path should be relative to repo root.
func BundleComponentBuildOpts(repoRoot string, minify bool) esbuild.BuildOptions {
	opts := browser.BrowserBuildOpts(repoRoot, minify)
	opts.Banner = defaultBanner()
	return opts
}
