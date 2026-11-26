package block

import "context"

type readOperationStoreContextKey struct{}

// readOperationContext carries the read-scoped store and its decoded
// block cache through the context.
type readOperationContext struct {
	store         StoreOps
	decodedBlocks *decodedBlockFrontCache
}

// WithReadOperationStore routes the current cursor operation through store.
// Storage implementations receive a context without this routing override so
// their own backing cursors remain bound to their underlying stores.
func WithReadOperationStore(ctx context.Context, store StoreOps) context.Context {
	if store == nil {
		return ctx
	}
	cache := newDecodedBlockFrontCache()
	if front := readOperationContextFromContext(ctx).frontCache(); front != nil {
		cache = front
	}
	return context.WithValue(ctx, readOperationStoreContextKey{}, &readOperationContext{
		store:         store,
		decodedBlocks: cache,
	})
}

// readStore returns the read-scoped store, or nil when op is nil.
func (op *readOperationContext) readStore() StoreOps {
	if op == nil {
		return nil
	}
	return op.store
}

// frontCache returns the read operation's decoded-block front cache, or nil
// when op is nil.
func (op *readOperationContext) frontCache() *decodedBlockFrontCache {
	if op == nil {
		return nil
	}
	return op.decodedBlocks
}

// withoutReadOperationStore ends routing at the selected storage boundary.
func withoutReadOperationStore(ctx context.Context) context.Context {
	if readOperationContextFromContext(ctx) == nil {
		return ctx
	}
	return context.WithValue(ctx, readOperationStoreContextKey{}, (*readOperationContext)(nil))
}

// readOperationContextFromContext returns the read operation context from
// the context, or nil.
func readOperationContextFromContext(ctx context.Context) *readOperationContext {
	op, _ := ctx.Value(readOperationStoreContextKey{}).(*readOperationContext)
	return op
}
