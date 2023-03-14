package dist_compiler

import (
	"bytes"
	"fmt"
)

// FormatEntrypoint formats the entrypoint code for the dist binary.
func FormatEntrypoint(
	appID string,
	distPlatformID string,
	staticPluginPkgNames,
	staticPluginPkgPaths,
	startPlugins []string,
) []byte {
	var out bytes.Buffer

	p := func(fmtStr string, args ...interface{}) {
		_, _ = fmt.Fprintf(&out, fmtStr, args...)
	}

	p("package main\n\n")
	p("import (\n")
	p("\tdist_entrypoint \"github.com/aperturerobotics/bldr/dist/entrypoint\"\n")
	p("\tplugin \"github.com/aperturerobotics/bldr/plugin\"\n")
	p("\t\"github.com/sirupsen/logrus\"\n\n")
	for i, pkgName := range staticPluginPkgNames {
		p("\t%s %q\n", pkgName, staticPluginPkgPaths[i])
	}
	p(")\n\n")

	p("var AppID = %q\n\n", appID)

	p("var DistPlatformID = %q\n\n", distPlatformID)

	p("var LogLevel = logrus.DebugLevel\n\n") // TODO

	p("var PluginManifests = []*plugin.StaticPlugin{\n")
	for _, pkgName := range staticPluginPkgNames {
		p("\t%s.StaticPlugin,\n", pkgName)
	}
	p("}\n\n")

	p("var StartPlugins = []string{\n")
	for _, startPluginID := range startPlugins {
		p("\t%q,\n", startPluginID)
	}
	p("}\n\n")

	p("func main() {\n")
	p("\tdist_entrypoint.Main(AppID, DistPlatformID, StartPlugins)\n")
	p("}\n")

	return out.Bytes()
}
