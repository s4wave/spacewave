import { useEffect, useRef, type ReactNode } from 'react'
import { cn } from '@s4wave/web/style/utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: { color: themeColors['--color-foreground-alt'] },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule3: { textOverflow: 'ellipsis', whiteSpace: 'nowrap', overflow: 'hidden' },
  rule4: {
    flexShrink: 0,
    fontSize: '10px',
    lineHeight: 1.5,
    color: themeColors['--color-foreground-alt'],
  },
  rule5: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule6: {
    display: 'flex',
    width: '100%',
    cursor: 'pointer',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    textAlign: 'left',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-background-tertiary'],
      },
    },
  },
})

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

const defaultCustomStatusLabels: Partial<Record<StatusListItemStatus, string>> =
  {}

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
  const prevStatusRef = useRef(status)
  const changed = prevStatusRef.current !== status

  useEffect(() => {
    prevStatusRef.current = status
  }, [status])

  return (
    <span
      className={
        /* TODO(stylex): global or unknown class: hdr-status-flash */ cn(
          'hdr-status-flash micro-ten font-bold',
          statusClassNames[status],
        )
      }
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
  statusLabels = defaultCustomStatusLabels,
  className,
  onItemClick,
}: StatusListProps) {
  const labels = { ...defaultStatusLabels, ...statusLabels }
  return (
    <div
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'flex-1 overflow-auto rounded border p-2 font-mono text-xs',
          'bg-background-secondary text-foreground',
          'border-ui-outline',
          className,
        )
      }
    >
      {items.length === 0 ? (
        <div sx={stylexStyles.rule1}>{emptyMessage}</div>
      ) : (
        items.map((item) => {
          const content = (
            <>
              <span sx={stylexStyles.rule2}>
                <StatusIndicator
                  status={item.status}
                  label={labels[item.status]}
                />
                <span sx={stylexStyles.rule3}>{item.label}</span>
              </span>
              {item.detail && (
                <span sx={stylexStyles.rule4}>{item.detail}</span>
              )}
            </>
          )
          if (!onItemClick) {
            return (
              <div key={item.id} sx={stylexStyles.rule5}>
                {content}
              </div>
            )
          }
          return (
            <button
              key={item.id}
              type="button"
              sx={stylexStyles.rule6}
              onClick={() => onItemClick(item)}
            >
              {content}
            </button>
          )
        })
      )}
    </div>
  )
}
