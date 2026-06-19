import type { KvKeyEntry } from '@s4wave/sdk/kv/kv.js'

// KvSortDirection is the key-list sort order.
export type KvSortDirection = 'asc' | 'desc'

// KvKeyRow is a key-list row: the entry plus its decoded text label.
export interface KvKeyRow extends KvKeyEntry {
  // label is the key rendered as UTF-8 text for display and filtering.
  label: string
}
