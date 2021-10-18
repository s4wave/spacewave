//go:build js
// +build js

package main

import (
	"syscall/js"

	"github.com/aperturerobotics/bldr/banner"
)

// formatBanner formats the full banner.
func formatBanner() string {
	return banner.FormatBanner()
}

// writeBanner writes the banner to the browser console.
func writeBanner() {
	defer func() {
		_ = recover()
	}()

	// write aperture banner
	js.Global().Get("console").Call(
		"log",
		"%c"+formatBanner(),
		"color:#ff3838;font-size:0.98em;font-family:monospace",
	)
}
