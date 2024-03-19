package bldr_dist_compiler

import (
	"fmt"
	"strings"

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

// AssetsFS contains embedded static assets.%s
//
//%s
var AssetsFS embed.FS

func main() {
	dist_entrypoint.Main(DistMeta, LogLevel, AssetsFS)
}
`

// FormatDistEntrypoint formats the embedded dist entrypoint code.
func FormatDistEntrypoint(
	meta *bldr_dist.DistMeta,
	embedAssetsFS []string,
) string {
	var goEmbedLine string
	if len(embedAssetsFS) != 0 {
		goEmbedLine = "go:embed " + strings.Join(embedAssetsFS, " ")
	} else {
		goEmbedLine = " [empty]"
	}

	return fmt.Sprintf(
		distEntrypointFmt,
		// DistMeta
		meta.MarshalB58(),
		// AssetsFS contents for go:embed
		goEmbedLine,
	)
}
