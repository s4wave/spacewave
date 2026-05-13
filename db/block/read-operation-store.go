package block

import "context"

type readOperationStoreContextKey struct{}

// WithReadOperationStore returns a context that routes block fetches through store.
func WithReadOperationStore(ctx context.Context, store StoreOps) context.Context {
	if store == nil {
		return ctx
	}
	return context.WithValue(ctx, readOperationStoreContextKey{}, store)
}

func readOperationStore(ctx context.Context) StoreOps {
	store, _ := ctx.Value(readOperationStoreContextKey{}).(StoreOps)
	return store
}
