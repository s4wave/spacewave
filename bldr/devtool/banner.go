package devtool

import (
	"os"

	fcolor "github.com/fatih/color"
	"github.com/s4wave/spacewave/bldr/banner"
)

// writeBanner writes the banner in red to os.stderr.
func writeBanner() {
	red := fcolor.New(fcolor.FgRed)
	red.Fprint(os.Stderr, banner.FormatBanner()+"\n")
}
