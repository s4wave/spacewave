//go:build js
// +build js

package main

import (
	"runtime"
	"syscall/js"
)

const apertureBanner = `
       d8888                           888
      d88888                           888                             
     d88P888                           888                             
    d88P 888 88888b.   .d88b.  888d888 888888 888  888 888d888 .d88b.  
   d88P  888 888 "88b d8P  Y8b 888P"   888    888  888 888P"  d8P  Y8b 
  d88P   888 888  888 88888888 888     888    888  888 888    88888888 
 d8888888888 888 d88P Y8b.     888     Y88b.  Y88b 888 888    Y8b.     
d88P     888 88888P"   "Y8888  888      "Y888  "Y88888 888     "Y8888  
             888                                                       
             888                                                       
             888     Welcome, user. `

// formatBanner formats the full banner.
func formatBanner() string {
	// versionInfo is the version info str
	versionInfo := "Bldr " + runtime.Version() + " on " + runtime.GOOS + "/" + runtime.GOARCH
	return apertureBanner + versionInfo
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
