//go:build !js

package bldr_dist_compiler

import (
	"slices"
	"strconv"
	"strings"

	bldr_cli_compiler "github.com/s4wave/spacewave/bldr/cli/compiler"
	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

// distEntrypointTemplate contains the generated distribution main package.
const distEntrypointTemplate = `package main

import (
	"embed"

__IMPORTS__	dist_entrypoint "github.com/s4wave/spacewave/bldr/dist/entrypoint"
	"github.com/sirupsen/logrus"
)

// DistMeta is the dist metadata encoded in b58.
// type: bldr_dist.DistMeta
var DistMeta = __META__

// LogLevel is the logging level to use.
var LogLevel = logrus.__LOG_LEVEL__

// AssetsFS contains embedded static assets.
//
//__EMBED__
var AssetsFS embed.FS

__COMMANDS__
func main() {
	__MAIN__
}
`

// FormatDistEntrypoint formats the embedded dist entrypoint code.
func FormatDistEntrypoint(
	meta *bldr_dist.DistMeta,
	embedAssetsFS []string,
	cliImports map[string]bldr_cli_compiler.CliImport,
	buildType bldr_manifest.BuildType,
	nativeBuild bool,
	nativeRunnerPackage string,
) string {
	logLevel := "DebugLevel"
	if buildType.IsRelease() {
		logLevel = "WarnLevel"
	}

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
		if nativeRunnerPackage != "" {
			importLines.WriteString("\tnative_runner " + strconv.Quote(nativeRunnerPackage) + "\n")
			mainCall = "dist_entrypoint.MainWithRunner(DistMeta, LogLevel, AssetsFS, cliCommands, native_runner.Run)"
		}
	}

	return strings.NewReplacer(
		"__IMPORTS__", importLines.String(),
		"__META__", strconv.Quote(meta.MarshalB58()),
		"__LOG_LEVEL__", logLevel,
		"__EMBED__", goEmbedLine,
		"__COMMANDS__", cliCommandsDecl,
		"__MAIN__", mainCall,
	).Replace(distEntrypointTemplate)
}
