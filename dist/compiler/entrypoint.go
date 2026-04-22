package bldr_dist_compiler

import (
	"fmt"
	"slices"
	"strings"

	bldr_dist "github.com/aperturerobotics/bldr/dist"
)

// distEntrypointFmt is the format for the dist entrypoint file.
const distEntrypointFmt = `package main

import (
	"embed"

%s	dist_entrypoint "github.com/aperturerobotics/bldr/dist/entrypoint"
	"github.com/sirupsen/logrus"
)

// DistMeta is the dist metadata encoded in b58.
// type: bldr_dist.DistMeta
var DistMeta = %q

// LogLevel is the logging level to use.
var LogLevel = logrus.DebugLevel

// AssetsFS contains embedded static assets.
//
//%s
var AssetsFS embed.FS

%s
func main() {
	%s
}
`

// FormatDistEntrypoint formats the embedded dist entrypoint code.
func FormatDistEntrypoint(
	meta *bldr_dist.DistMeta,
	embedAssetsFS []string,
	cliImports map[string]string,
	nativeBuild bool,
) string {
	var goEmbedLine string
	if len(embedAssetsFS) != 0 {
		goEmbedLine = "go:embed " + strings.Join(embedAssetsFS, " ")
	} else {
		goEmbedLine = " [empty]"
	}

	var importLines strings.Builder
	var cliCommandsDecl string
	if nativeBuild {
		importLines.WriteString("\tcli_entrypoint \"github.com/aperturerobotics/bldr/cli/entrypoint\"\n")
	}
	if len(cliImports) != 0 {
		importPkgs := make([]string, 0, len(cliImports))
		for pkg := range cliImports {
			importPkgs = append(importPkgs, pkg)
		}
		slices.Sort(importPkgs)
		for _, pkg := range importPkgs {
			importLines.WriteString("\t")
			importLines.WriteString(cliImports[pkg])
			importLines.WriteString(" ")
			importLines.WriteString(fmt.Sprintf("%q", pkg))
			importLines.WriteString("\n")
		}

		aliases := make([]string, 0, len(cliImports))
		for _, alias := range cliImports {
			aliases = append(aliases, alias)
		}
		slices.Sort(aliases)
		builders := make([]string, 0, len(aliases))
		for _, alias := range aliases {
			builders = append(builders, alias+".NewCliCommands")
		}
		cliCommandsDecl = "// cliCommands are the native CLI command builders.\n" +
			"var cliCommands = []cli_entrypoint.BuildCommandsFunc{" +
			strings.Join(builders, ", ") + "}\n"
	}
	if nativeBuild && len(cliImports) == 0 {
		cliCommandsDecl = "// cliCommands are the native CLI command builders.\n" +
			"var cliCommands []cli_entrypoint.BuildCommandsFunc\n"
	}

	mainCall := "dist_entrypoint.Main(DistMeta, LogLevel, AssetsFS)"
	if nativeBuild {
		mainCall = "dist_entrypoint.Main(DistMeta, LogLevel, AssetsFS, cliCommands)"
	}

	return fmt.Sprintf(
		distEntrypointFmt,
		importLines.String(),
		meta.MarshalB58(),
		goEmbedLine,
		cliCommandsDecl,
		mainCall,
	)
}
