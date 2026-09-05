//go:build !js

package bldr_dist_compiler

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	bldr_cli_compiler "github.com/s4wave/spacewave/bldr/cli/compiler"
	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
)

// distEntrypointFmt is the format for the dist entrypoint file.
const distEntrypointFmt = `package main

import (
	"embed"

%s	dist_entrypoint "github.com/s4wave/spacewave/bldr/dist/entrypoint"
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
	cliImports map[string]bldr_cli_compiler.CliImport,
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
		importLines.WriteString("\tcli_entrypoint \"github.com/s4wave/spacewave/bldr/cli/entrypoint\"\n")
	}
	if len(cliImports) != 0 {
		importPkgs := make([]string, 0, len(cliImports))
		for pkg := range cliImports {
			importPkgs = append(importPkgs, pkg)
		}
		slices.Sort(importPkgs)
		for _, pkg := range importPkgs {
			importLines.WriteString("\t")
			importLines.WriteString(cliImports[pkg].Alias)
			importLines.WriteString(" ")
			importLines.WriteString(strconv.Quote(pkg))
			importLines.WriteString("\n")
		}

		imports := make([]bldr_cli_compiler.CliImport, 0, len(cliImports))
		var needsBroker bool
		for _, ci := range cliImports {
			imports = append(imports, ci)
			needsBroker = needsBroker || ci.TakesYieldBroker
		}
		slices.SortFunc(imports, func(a, b bldr_cli_compiler.CliImport) int { return strings.Compare(a.Alias, b.Alias) })
		builders := make([]string, 0, len(imports))
		for _, ci := range imports {
			builders = append(builders, ci.CommandBuilder(meta.GetProjectId(), "yieldBroker"))
		}
		if needsBroker {
			importLines.WriteString("\taperture_cli \"github.com/aperturerobotics/cli\"\n")
			importLines.WriteString("\tyield_policy \"github.com/s4wave/spacewave/core/resource/listener/yieldpolicy\"\n")
			cliCommandsDecl = "var yieldBroker = yield_policy.NewBroker()\n\n"
		}
		cliCommandsDecl += "// cliCommands are the native CLI command builders.\n" +
			"var cliCommands = []cli_entrypoint.BuildCommandsFunc{" +
			strings.Join(builders, ", ") + "}\n"
	}
	if nativeBuild && len(cliImports) == 0 {
		cliCommandsDecl += "// cliCommands are the native CLI command builders.\n" +
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
