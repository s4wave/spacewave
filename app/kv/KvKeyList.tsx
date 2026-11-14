import { LuKey } from 'react-icons/lu'

import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import type { ListItem, ListStateHandle } from '@s4wave/web/ui/list'
import { Table, type TableColumn } from '@s4wave/web/ui/table/Table.js'

import type { KvKeyRow } from './kv-viewer.js'

// keyColumns are the key list's columns: the key, ordered as the store orders
// it, and its value's size, which sheds first.
const keyColumns: TableColumn<KvKeyRow>[] = [
  {
    key: 'key',
    label: 'Key',
    kind: 'id',
    text: (row) => row.label,
    compare: (a, b) => (a.label < b.label ? -1 : a.label > b.label ? 1 : 0),
  },
  {
    key: 'size',
    label: 'Size',
    minWidth: 3.5,
    end: true,
    text: (row) => `${row.byteLength} B`,
    compare: (a, b) => a.byteLength - b.byteLength,
  },
]

interface KvKeyListProps {
  // rows are the keys the prefix filter leaves, by label.
  rows: ListItem<KvKeyRow>[]
  // state is the key list's selection, focus and sort.
  state: ListStateHandle
  // filtered is true while a prefix filters the keys.
  filtered: boolean
}

// KvKeyList is the store's keys as a Table: each key with its value's size,
// sorted by either.
export function KvKeyList({ rows, state, filtered }: KvKeyListProps) {
  return (
    <Table
      rows={rows}
      columns={keyColumns}
      state={state}
      label="Keys"
      className="min-h-0 flex-1"
      placeholder={
        <EmptyState
          variant="compact"
          icon={<LuKey className="text-foreground-alt size-5" />}
          title={filtered ? 'No matching keys' : 'No keys yet'}
          description={
            filtered
              ? 'No keys match this prefix.'
              : 'Add a key to populate this store.'
          }
        />
      }
    />
  )
}
