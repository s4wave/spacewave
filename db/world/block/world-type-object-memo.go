package world_block

// TypeObjectEnsured reports whether the type object key is known to exist for
// the current write transaction. Returns false once the transaction rebuilds
// its block state (SetBlockTransaction, Discard).
func (t *WorldState) TypeObjectEnsured(typeObjectKey string) bool {
	_, ok := t.typeObjectMemo[typeObjectKey]
	return ok
}

// MarkTypeObjectEnsured records that the type object key exists for the rest of
// the current write transaction. Read-only states do not memo: their type
// objects cannot be created and the state is discarded rather than committed.
func (t *WorldState) MarkTypeObjectEnsured(typeObjectKey string) {
	if !t.write {
		return
	}
	if t.typeObjectMemo == nil {
		t.typeObjectMemo = make(map[string]struct{})
	}
	t.typeObjectMemo[typeObjectKey] = struct{}{}
}

// forgetTypeObject drops an object key from the type memo when the object is
// deleted or renamed, so a later EnsureTypeExists re-creates it rather than
// trusting stale transaction-local knowledge.
func (t *WorldState) forgetTypeObject(key string) {
	if t.typeObjectMemo == nil {
		return
	}
	delete(t.typeObjectMemo, key)
}
