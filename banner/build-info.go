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
	goarch := runtime.GOARCH
	if goarch == "ecmascript" {
		goarch = "js"
	}
	info := buildInfo{
		goVersion: runtime.Version(),
		goos:      runtime.GOOS,
		goarch:    goarch,
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		info.mainVersion = bi.Main.Version
	}
	return info
}

func (b buildInfo) normalizedMainVersion() string {
	if b.mainVersion == "" || b.mainVersion == "(devel)" {
		return "dev"
	}
	return b.mainVersion
}

func (b buildInfo) runtimeLabel() string {
	return b.goVersion + " on " + b.goos + "/" + b.goarch
}

func (b buildInfo) cornerLabel() string {
	if b.goVersion == "" {
		return b.normalizedMainVersion()
	}
	return b.normalizedMainVersion() + "@" + b.goVersion
}
