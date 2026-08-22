package block

import "context"

type readOperationStoreContextKey struct{}

// readOperationContext carries the read-scoped store and its decoded
// block cache through the context.
type readOperationContext struct {
	store         StoreOps
	decodedBlocks *decodedBlockFrontCache
}

// WithReadOperationStore returns a context that routes block fetches through store.
func WithReadOperationStore(ctx context.Context, store StoreOps) context.Context {
	if store == nil {
		return ctx
	}
	cache := newDecodedBlockFrontCache()
	if op := readOperationContextFromContext(ctx); op != nil && op.decodedBlocks != nil {
		cache = op.decodedBlocks
	}
	return context.WithValue(ctx, readOperationStoreContextKey{}, &readOperationContext{
		store:         store,
		decodedBlocks: cache,
	})
}

// readOperationStore returns the read-scoped store from the context, or
// nil.
func readOperationStore(ctx context.Context) StoreOps {
	op := readOperationContextFromContext(ctx)
	if op == nil {
		return nil
	}
	return op.store
}

// readOperationContextFromContext returns the read operation context from
// the context, or nil.
func readOperationContextFromContext(ctx context.Context) *readOperationContext {
	op, _ := ctx.Value(readOperationStoreContextKey{}).(*readOperationContext)
	return op
}
