package unixfs

import "context"

// fileCreateExpectationKey identifies the one publication precondition.
type fileCreateExpectationKey struct{}

// WithExclusiveFileCreate asks the filesystem to publish only at a vacant name.
func WithExclusiveFileCreate(ctx context.Context) context.Context {
	return context.WithValue(ctx, fileCreateExpectationKey{}, true)
}

// IsExclusiveFileCreate reports whether file creation requires a vacant name.
func IsExclusiveFileCreate(ctx context.Context) bool {
	exclusive, _ := ctx.Value(fileCreateExpectationKey{}).(bool)
	return exclusive
}

// WithExpectedFileVersion asks the filesystem to replace only the nonempty version.
func WithExpectedFileVersion(ctx context.Context, version []byte) context.Context {
	return context.WithValue(ctx, fileCreateExpectationKey{}, version)
}

// ExpectedFileVersion returns the version a caller requires at publication, if any.
func ExpectedFileVersion(ctx context.Context) []byte {
	version, _ := ctx.Value(fileCreateExpectationKey{}).([]byte)
	return version
}

// ConditionalFilePublisher marks operations that enforce file upload preconditions.
type ConditionalFilePublisher interface {
	// SupportsConditionalFilePublication declares support for the publication context.
	SupportsConditionalFilePublication()
}
