package npm

// GOOSToNodePlatform maps a Go GOOS value to the corresponding Node.js
// process.platform value used by @electron/get and other npm install-time
// platform hooks. Returns "" for GOOS values with no Node equivalent.
func GOOSToNodePlatform(goos string) string {
	switch goos {
	case "windows":
		return "win32"
	case "darwin", "linux", "freebsd", "openbsd", "aix", "sunos":
		return goos
	default:
		return ""
	}
}

// GOARCHToNodeArch maps a Go GOARCH value to the corresponding Node.js
// process.arch value. Electron ships redistributables for a subset; values
// not in that subset are returned as-is and will surface as a download
// failure from @electron/get rather than a silent host-arch fallback.
func GOARCHToNodeArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "x64"
	case "386":
		return "ia32"
	case "arm64", "arm", "mips", "mipsel", "ppc64", "s390x":
		return goarch
	default:
		return goarch
	}
}
