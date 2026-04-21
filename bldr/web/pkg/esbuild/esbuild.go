//go:build !js

package web_pkg_esbuild

import (
	"strconv"
	"strings"

	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	web_pkg_external "github.com/s4wave/spacewave/bldr/web/pkg/external"
)

// jsExtensions is the set of file extensions that map to .mjs in output.
var jsExtensions = map[string]bool{
	".js": true, ".cjs": true, ".mjs": true,
	".jsx": true, ".ts": true, ".tsx": true,
}

// isJSExtension returns true if the extension is a JS-like extension.
func isJSExtension(ext string) bool {
	return jsExtensions[ext]
}

// NewImportBannerShim generates an esbuild require() shim for cjs module compatibility.
//
// This is a hack to work around issues with require() in esbuild.
// The require() function is not asynchronous but import() is.
// xfrmImport can be set to override the import path for a package.
//
// https://github.com/evanw/esbuild/issues/1921
func NewImportBannerShim(pkgs []string, minify bool, xfrmImport func(pkg string) string) string {
	var sb strings.Builder
	// write import statements
	// import * as __bldr_react from 'react';
	pkgVarNames := make([]string, len(pkgs))
	for i, pkg := range pkgs {
		// clean pkg name for use as variable
		pkgVarName := strings.ReplaceAll(pkg, "@", "_")
		pkgVarName = strings.ReplaceAll(pkgVarName, "/", "_")
		pkgVarName = strings.ReplaceAll(pkgVarName, "-", "_")

		// prepend __bldr_ to variable name to deconflict
		pkgVarName = "__bldr_" + pkgVarName
		pkgVarNames[i] = pkgVarName

		_, _ = sb.WriteString("import * as ")
		_, _ = sb.WriteString(pkgVarName)
		_, _ = sb.WriteString(" from ")
		impPkg := pkg
		if xfrmImport != nil {
			impPkg = xfrmImport(impPkg)
			if impPkg == "" {
				impPkg = pkg
			}
		}
		_, _ = sb.WriteString(strconv.Quote(impPkg))
		_, _ = sb.WriteString(";\n")
	}

	// write require function implementation
	_, _ = sb.WriteString("const require = (pkgName) => {\n")
	_, _ = sb.WriteString("  switch (pkgName) {\n")
	for i, pkg := range pkgs {
		_, _ = sb.WriteString("  case ")
		_, _ = sb.WriteString(strconv.Quote(pkg))
		_, _ = sb.WriteString(":\n")
		_, _ = sb.WriteString("    return ")
		_, _ = sb.WriteString(pkgVarNames[i])
		_, _ = sb.WriteString(";\n")
	}
	_, _ = sb.WriteString("  default:\n")
	_, _ = sb.WriteString("    throw Error('Dynamic require of \"' + pkgName + '\" is not supported');\n")
	_, _ = sb.WriteString("  }\n};\n")

	// minify
	code := sb.String()
	result := esbuild_api.Transform(code, esbuild_api.TransformOptions{
		Target:    esbuild_api.ES2022,
		Sourcemap: esbuild_api.SourceMapNone,
		Platform:  esbuild_api.PlatformBrowser,

		MinifyWhitespace:  minify,
		MinifySyntax:      minify,
		MinifyIdentifiers: false,
	})
	return string(result.Code)
}

// FixEsbuildIssue1921 fixes externalized esbuild imports failing with compiled commonjs modules.
//
// https://github.com/evanw/esbuild/issues/1921
func FixEsbuildIssue1921(opts *esbuild_api.BuildOptions) {
	if opts.Banner == nil {
		opts.Banner = make(map[string]string, 1)
	}
	old := opts.Banner["js"]
	if len(old) != 0 {
		old += "\n"
	}
	opts.Banner["js"] = old + NewImportBannerShim(web_pkg_external.BldrExternal, opts.MinifySyntax, nil)
}
