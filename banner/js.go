//go:build js

package banner

import "syscall/js"

// WriteToConsole writes the banner to the js console.
func WriteToConsole() {
	// ignore panics here
	defer func() {
		_ = recover()
	}()

	// write aperture banner
	buildInfo := getBuildInfo()
	js.Global().Set(
		"__BLDR_BUILD_INFO__",
		js.ValueOf(map[string]any{
			"mainVersion":  buildInfo.mainVersion,
			"goVersion":    buildInfo.goVersion,
			"goos":         buildInfo.goos,
			"goarch":       buildInfo.goarch,
			"runtimeLabel": buildInfo.runtimeLabel(),
		}),
	)
	js.Global().Get("console").Call(
		"log",
		"%c"+FormatBanner(),
		"color:#ff3838;font-size:0.98em;font-family:monospace",
	)

	/*
		js.Global().Get("console").Call(
			"log",
			"%c"+"Oh. It's you... It's been a long time. How have you been?",
			// "color:#ff9a00;font-size:1.02em;font-family:monospace",
			"color:#27a7d8;font-size:0.8em;font-family:monospace",
		)
	*/
}
