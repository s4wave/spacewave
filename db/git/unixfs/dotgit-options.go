package unixfs_git

import "github.com/pkg/errors"

// ErrDotGitWriteNotImplemented is returned by writable .git cursors before semantic writes are implemented.
var ErrDotGitWriteNotImplemented = errors.New("dotgit write support is not implemented")

// DotGitFSCursorChangeSource registers coarse invalidation callbacks for a repo.
type DotGitFSCursorChangeSource interface {
	AddDotGitChangeCb(cb func()) func()
}

type dotGitFSCursorOptions struct {
	writable     bool
	changeSource DotGitFSCursorChangeSource
	releaseFn    func()
}

// DotGitFSCursorOption configures a .git filesystem cursor.
type DotGitFSCursorOption func(*dotGitFSCursorOptions)

// WithDotGitWritable configures whether the .git cursor accepts write attempts.
func WithDotGitWritable(writable bool) DotGitFSCursorOption {
	return func(opts *dotGitFSCursorOptions) {
		opts.writable = writable
	}
}

// WithDotGitChangeSource configures the repo-level invalidation source.
func WithDotGitChangeSource(changeSource DotGitFSCursorChangeSource) DotGitFSCursorOption {
	return func(opts *dotGitFSCursorOptions) {
		opts.changeSource = changeSource
	}
}

// WithDotGitReleaseFn configures cleanup called once when the cursor releases.
func WithDotGitReleaseFn(releaseFn func()) DotGitFSCursorOption {
	return func(opts *dotGitFSCursorOptions) {
		opts.releaseFn = releaseFn
	}
}
