package banner

import (
	"runtime"
	"runtime/debug"
)

type buildInfo struct {
	mainVersion string
	goVersion   string
	goos        string
	goarch      string
}

func getBuildInfo() buildInfo {
	// Normalize the architecture name for buildInfo.runtimeLabel.
	goarch := runtime.GOARCH
	if goarch == "ecmascript" {
		goarch = "js"
	}

	// Capture the Go version, operating system, and normalized architecture.
	info := buildInfo{
		goVersion: runtime.Version(),
		goos:      runtime.GOOS,
		goarch:    goarch,
	}

	// Add the module version when Go exposes build metadata.
	if bi, ok := debug.ReadBuildInfo(); ok {
		info.mainVersion = bi.Main.Version
	}
	return info
}

func (b buildInfo) runtimeLabel() string {
	return b.goVersion + " on " + b.goos + "/" + b.goarch
}
