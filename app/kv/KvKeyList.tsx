import { useCallback } from 'react'
import { LuKey } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'

import type { KvKeyRow } from './kv-viewer.js'

interface KvKeyListProps {
  rows: KvKeyRow[]
  selectedKey: string | null
  onSelectKey: (row: KvKeyRow) => void
  filtered: boolean
}

// KvKeyList renders the scrollable key list with a byte-length badge per row.
export function KvKeyList({
  rows,
  selectedKey,
  onSelectKey,
  filtered,
}: KvKeyListProps) {
  if (rows.length === 0) {
    return (
      <div className="flex-1 overflow-auto">
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
      </div>
    )
  }

  return (
    <div role="listbox" aria-label="Keys" className="flex-1 overflow-auto">
      {rows.map((row) => (
        <KvKeyListRow
          key={row.label}
          row={row}
          selected={row.label === selectedKey}
          onSelectKey={onSelectKey}
        />
      ))}
    </div>
  )
}

interface KvKeyListRowProps {
  row: KvKeyRow
  selected: boolean
  onSelectKey: (row: KvKeyRow) => void
}

function KvKeyListRow({ row, selected, onSelectKey }: KvKeyListRowProps) {
  const selectKey = useCallback(() => {
    onSelectKey(row)
  }, [onSelectKey, row])

  return (
    <button
      type="button"
      role="option"
      aria-selected={selected}
      onClick={selectKey}
      className={cn(
        'flex w-full items-center gap-2 px-3 py-1.5 text-left transition-colors',
        'hover:bg-foreground/5',
        selected && 'bg-brand/10 hover:bg-brand/15',
      )}
    >
      <span className="text-foreground min-w-0 flex-1 truncate font-mono text-xs">
        {row.label}
      </span>
      <span className="text-foreground-alt/50 shrink-0 text-[0.6rem] tabular-nums">
        {row.byteLength} B
      </span>
    </button>
  )
}
