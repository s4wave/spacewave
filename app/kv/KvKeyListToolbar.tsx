import { useCallback } from 'react'
import { LuArrowDownAZ, LuArrowUpAZ } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { Input } from '@s4wave/web/ui/input.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'

import type { KvSortDirection } from './kv-viewer.js'

interface KvKeyListToolbarProps {
  prefix: string
  onPrefixChange: (prefix: string) => void
  sortDirection: KvSortDirection
  onToggleSortDirection: () => void
}

// KvKeyListToolbar renders the prefix filter input and sort direction toggle
// for the key list. Both controls drive the watched list reactively.
export function KvKeyListToolbar({
  prefix,
  onPrefixChange,
  sortDirection,
  onToggleSortDirection,
}: KvKeyListToolbarProps) {
  const handlePrefixChange = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      onPrefixChange(event.target.value)
    },
    [onPrefixChange],
  )

  return (
    <div className="border-foreground/8 flex shrink-0 items-center gap-2 border-b px-2 py-1.5">
      <Input
        value={prefix}
        onChange={handlePrefixChange}
        placeholder="Filter by key prefix"
        aria-label="Filter by key prefix"
        className={cn(
          'border-foreground/10 bg-background/20 h-7 flex-1 text-xs',
          'placeholder:text-foreground-alt/40',
          'focus-visible:border-brand/50 focus-visible:ring-brand/15',
        )}
      />
      <DashboardButton
        icon={
          sortDirection === 'asc' ? (
            <LuArrowDownAZ className="h-3.5 w-3.5" />
          ) : (
            <LuArrowUpAZ className="h-3.5 w-3.5" />
          )
        }
        onClick={onToggleSortDirection}
        aria-label={
          sortDirection === 'asc'
            ? 'Sort keys descending'
            : 'Sort keys ascending'
        }
        title={sortDirection === 'asc' ? 'Ascending' : 'Descending'}
      />
    </div>
  )
}
