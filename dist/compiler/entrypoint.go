package bldr_dist_compiler

import (
	"fmt"

	bldr_dist "github.com/aperturerobotics/bldr/dist"
)

// distEntrypointFmt is the format for the dist entrypoint file.
const distEntrypointFmt = `package main

import (
	"embed"

	dist_entrypoint "github.com/aperturerobotics/bldr/dist/entrypoint"
	"github.com/sirupsen/logrus"
)

// DistMeta is the dist metadata encoded in b58.
// type: bldr_dist.DistMeta
var DistMeta = %q

// LogLevel is the logging level to use.
var LogLevel = logrus.DebugLevel

// StaticFS contains embedded static assets.
//
//go:embed config-set.bin volume.kvfile
var StaticFS embed.FS

func main() {
	dist_entrypoint.Main(DistMeta, LogLevel, StaticFS)
}
`

// FormatDistEntrypoint formats the embedded dist entrypoint code.
func FormatDistEntrypoint(
	meta *bldr_dist.DistMeta,
) string {
	return fmt.Sprintf(distEntrypointFmt, meta.MarshalB58())
}
