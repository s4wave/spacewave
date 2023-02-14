//go:build js
// +build js

package browser

import "github.com/blang/semver"

// ControllerID is the browser runtime controller ID.
const ControllerID = "bldr/target/browser"

// RuntimeID is the runtime identifier
const RuntimeID = "browser"

// Version is the version of the runtime implementation.
var Version = semver.MustParse("0.0.1")
