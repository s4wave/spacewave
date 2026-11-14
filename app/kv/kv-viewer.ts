import type { KvKeyEntry } from '@s4wave/sdk/kv/kv.js'

// KvKeyRow is a key-list row: the entry plus its decoded text label.
export interface KvKeyRow extends KvKeyEntry {
  // label is the key rendered as UTF-8 text for display and filtering.
  label: string
}
