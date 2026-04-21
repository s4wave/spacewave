package unixfs_iofs

// FSOption is a option passed to NewFS
type FSOption func(fs *FS)

// WithIgnorePath ignores the path passed to Open.
// All ops are applied to the root fs handle, ignoring the passed path.
func WithIgnorePath() FSOption {
	return func(fs *FS) {
		fs.ignorePath = true
	}
}
