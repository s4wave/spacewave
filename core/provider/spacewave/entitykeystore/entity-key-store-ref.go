package entitykeystore

import "sync"

// EntityKeyStoreRef is a retention ref for unlocked entity keys.
type EntityKeyStoreRef struct {
	// store owns the retained keys.
	store *EntityKeyStore
	// once releases the retention at most once.
	once sync.Once
}

// Release releases a retention ref.
func (r *EntityKeyStoreRef) Release() {
	if r == nil || r.store == nil {
		return
	}
	r.once.Do(func() {
		r.store.releaseRef()
	})
}
