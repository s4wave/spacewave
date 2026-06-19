//go:build !js

package devtool

import (
	"io"
	"os"

	fcolor "github.com/fatih/color"
	"github.com/s4wave/spacewave/bldr/banner"
)

func writeBannerTo(w io.Writer) {
	red := fcolor.New(fcolor.FgRed)
	red.Fprint(w, banner.FormatBanner()+"\n")
}

// writeBanner writes the banner in red to os.stderr.
func writeBanner() {
	writeBannerTo(os.Stderr)
}
