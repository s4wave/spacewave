package unixfs

import "context"

// exclusiveFileCreateKey identifies the file creation policy.
type exclusiveFileCreateKey struct{}

// WithExclusiveFileCreate asks the remote filesystem to publish only at a vacant name.
func WithExclusiveFileCreate(ctx context.Context) context.Context {
	return context.WithValue(ctx, exclusiveFileCreateKey{}, true)
}

// IsExclusiveFileCreate reports whether file creation requires a vacant name.
func IsExclusiveFileCreate(ctx context.Context) bool {
	exclusive, _ := ctx.Value(exclusiveFileCreateKey{}).(bool)
	return exclusive
}

// ExclusiveFileCreator marks operations that enforce exclusive file publication.
type ExclusiveFileCreator interface {
	// SupportsExclusiveFileCreate declares support for the request context's creation rule.
	SupportsExclusiveFileCreate()
}
