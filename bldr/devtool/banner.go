package devtool

import (
	"os"

	"github.com/s4wave/spacewave/bldr/banner"
	fcolor "github.com/fatih/color"
)

// writeBanner writes the banner in red to os.stderr.
func writeBanner() {
	red := fcolor.New(fcolor.FgRed)
	red.Fprint(os.Stderr, banner.FormatBanner()+"\n")
}
