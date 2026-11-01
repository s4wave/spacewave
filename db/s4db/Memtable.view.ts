// MemtableState exposes the copy owner and each caller's source epoch.
type MemtableState = { owner: number; epoch: number }

// title states the protected copy operation.
export const title = 'Published memtable copies serialize their COW metadata'

// state shows the mutable metadata, not the immutable tree nodes.
export const state = (v: MemtableState) => `owner ${v.owner}, epoch ${v.epoch}`

// step keeps the caller visible in the transition.
export const step = (action: string) => action
