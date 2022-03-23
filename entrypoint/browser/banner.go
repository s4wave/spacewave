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

	// clever note to anyone watching
	/*
		js.Global().Get("console").Call(
			"log",
			"%c"+"Oh. It's you... It's been a long time. How have you been?",
			// "color:#ff9a00;font-size:1.02em;font-family:monospace",
			"color:#27a7d8;font-size:0.8em;font-family:monospace",
		)
	*/
}
