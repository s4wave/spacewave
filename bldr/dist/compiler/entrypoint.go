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

__COMMANDS__// main is the main entrypoint.
func main() {
	__MAIN__
}
`

// FormatDistEntrypoint formats the embedded dist entrypoint code.
//
// When composePackage is set, main composes the host process from its Compose
// function; native builds add the cliImports command builders to it.
func FormatDistEntrypoint(
	meta *bldr_dist.DistMeta,
	embedAssetsFS []string,
	cliImports map[string]bldr_cli_compiler.CliImport,
	buildType bldr_manifest.BuildType,
	nativeBuild bool,
	nativeRunnerPackage string,
	composePackage string,
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

	// Compose the host process from the project, or start from nothing.
	var importLines strings.Builder
	var mainLines []string
	if composePackage != "" {
		importLines.WriteString("\tproject_compose " + strconv.Quote(composePackage) + "\n")
		mainLines = append(mainLines, "composition := project_compose.Compose()")
	} else {
		importLines.WriteString("\tcompose \"github.com/s4wave/spacewave/bldr/entrypoint/compose\"\n")
		mainLines = append(mainLines, "composition := &compose.Composition{}")
	}

	// Native builds add the command builders of each CLI package.
	var cliCommandsDecl string
	if nativeBuild && len(cliImports) != 0 {
		importLines.WriteString("\tcli_entrypoint \"github.com/s4wave/spacewave/bldr/cli/entrypoint\"\n")
		importPkgs := make([]string, 0, len(cliImports))
		for pkg := range cliImports {
			importPkgs = append(importPkgs, pkg)
		}
		slices.Sort(importPkgs)
		builders := make([]string, 0, len(importPkgs))
		for _, pkg := range importPkgs {
			alias := cliImports[pkg].Alias
			importLines.WriteString("\t" + alias + " " + strconv.Quote(pkg) + "\n")
			builders = append(builders, alias+".NewCliCommands")
		}
		cliCommandsDecl = "// cliCommands are the native CLI command builders.\n" +
			"var cliCommands = []cli_entrypoint.BuildCommandsFunc{" +
			strings.Join(builders, ", ") + "}\n\n"
		mainLines = append(mainLines, "composition.Commands = append(composition.Commands, cliCommands...)")
	}

	mainCall := "dist_entrypoint.Main(DistMeta, LogLevel, AssetsFS, composition)"
	if nativeBuild && nativeRunnerPackage != "" {
		importLines.WriteString("\tnative_runner " + strconv.Quote(nativeRunnerPackage) + "\n")
		mainCall = "dist_entrypoint.MainWithRunner(DistMeta, LogLevel, AssetsFS, composition, native_runner.Run)"
	}
	mainLines = append(mainLines, mainCall)

	return strings.NewReplacer(
		"__IMPORTS__", importLines.String(),
		"__META__", strconv.Quote(meta.MarshalB58()),
		"__LOG_LEVEL__", logLevel,
		"__EMBED__", goEmbedLine,
		"__COMMANDS__", cliCommandsDecl,
		"__MAIN__", strings.Join(mainLines, "\n\t"),
	).Replace(distEntrypointTemplate)
}
