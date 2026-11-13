package cli_entrypoint

import "runtime/debug"

// sourceRevision is the source revision a build names with
// -ldflags "-X github.com/s4wave/spacewave/bldr/cli/entrypoint.sourceRevision=<rev>".
// A build from a tree without Git metadata, such as a remote build mirror,
// has no VCS stamp and sets it this way.
var sourceRevision string

// SourceRevision returns the source revision the binary was built from: the
// linker-set sourceRevision, else the Go toolchain's VCS stamp with a
// "+dirty" suffix when the tree had uncommitted changes. It returns "" when
// the build recorded neither.
func SourceRevision() string {
	// Prefer the revision the build named explicitly.
	if sourceRevision != "" {
		return sourceRevision
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var rev, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if rev != "" && modified == "true" {
		rev += "+dirty"
	}
	return rev
}
