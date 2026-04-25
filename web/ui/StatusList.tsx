import { useState, type ReactNode } from 'react'
import { cn } from '@s4wave/web/style/utils.js'

// StatusListItemStatus defines the status of an item.
export type StatusListItemStatus = 'success' | 'error' | 'pending' | 'none'

// StatusListItem represents a single item in the list.
export interface StatusListItem {
  // id is a unique identifier for the item.
  id: string
  // label is the display text for the item.
  label: string
  // status is the current status of the item.
  status: StatusListItemStatus
  // detail is an optional detail string shown on the right.
  detail?: string
}

// StatusListProps are the props for the StatusList component.
export interface StatusListProps {
  // items is the array of items to display.
  items: StatusListItem[]
  // emptyMessage is shown when items is empty.
  emptyMessage?: ReactNode
  // statusLabels are optional custom labels for each status.
  statusLabels?: Partial<Record<StatusListItemStatus, string>>
  // className is an optional CSS class for the container.
  className?: string
  // onItemClick is an optional click handler for items.
  onItemClick?: (item: StatusListItem) => void
}

const defaultStatusLabels: Record<StatusListItemStatus, string> = {
  success: 'PASS',
  error: 'FAIL',
  pending: '....',
  none: '----',
}

const statusClassNames: Record<StatusListItemStatus, string> = {
  success: 'text-success',
  error: 'text-error',
  pending: 'text-warning',
  none: 'text-foreground-alt',
}

// StatusIndicator renders a status label and triggers the HDR flash animation
// when the status value changes.
function StatusIndicator({
  status,
  label,
}: {
  status: StatusListItemStatus
  label: string
}) {
  const [prevStatus, setPrevStatus] = useState(status)
  const changed = prevStatus !== status
  if (changed) {
    setPrevStatus(status)
  }

  return (
    <span
      className={cn(
        'hdr-status-flash text-[10px] font-bold',
        statusClassNames[status],
      )}
      {...(changed ? { 'data-status-changed': '' } : {})}
    >
      {label}
    </span>
  )
}

// StatusList displays a scrollable list of items with status indicators.
export function StatusList({
  items,
  emptyMessage = 'No items',
  statusLabels = {},
  className,
  onItemClick,
}: StatusListProps) {
  const labels = { ...defaultStatusLabels, ...statusLabels }

  return (
    <div
      className={cn(
        'flex-1 overflow-auto rounded border p-2 font-mono text-xs',
        'bg-background-secondary text-foreground',
        'border-ui-outline',
        className,
      )}
    >
      {items.length === 0 ?
        <div className="text-foreground-alt">{emptyMessage}</div>
      : items.map((item) => (
          <div
            key={item.id}
            className={cn(
              'flex items-center justify-between gap-2 py-0.5',
              onItemClick && 'hover:bg-background-tertiary cursor-pointer',
            )}
            onClick={onItemClick ? () => onItemClick(item) : undefined}
          >
            <div className="flex items-center gap-1.5">
              <StatusIndicator
                status={item.status}
                label={labels[item.status]}
              />
              <span className="truncate">{item.label}</span>
            </div>
            {item.detail && (
              <span className="text-foreground-alt shrink-0 text-[10px]">
                {item.detail}
              </span>
            )}
          </div>
        ))
      }
    </div>
  )
}
