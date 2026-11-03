import { keyframeNames } from '../style/tokens.stylex.js'
import type { StyleXStyles } from '@stylexjs/stylex'
import { useEffect, useRef, type ReactNode } from 'react'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const styles = stylex.create({
  indicator: {
    '--tw-font-weight': 'var(--font-weight-bold)',
    dynamicRangeLimit: 'no-limit',
    animationDelay: { default: null, ':is([data-status-changed])': '0s' },
    animationDirection: {
      default: null,
      ':is([data-status-changed])': 'normal',
    },
    animationDuration: { default: null, ':is([data-status-changed])': '.2s' },
    animationFillMode: { default: null, ':is([data-status-changed])': 'none' },
    animationIterationCount: { default: null, ':is([data-status-changed])': 1 },
    animationName: {
      default: null,
      ':is([data-status-changed])': keyframeNames['hdr-flash'],
    },
    animationPlayState: {
      default: null,
      ':is([data-status-changed])': 'running',
    },
    animationTimeline: { default: null, ':is([data-status-changed])': 'auto' },
    animationTimingFunction: {
      default: null,
      ':is([data-status-changed])': 'ease-out',
    },
    fontSize: '10px',
    fontWeight: 'var(--font-weight-bold)',
    lineHeight: 1.5,
  },
  success: { color: themeColors['--color-success'] },
  error: { color: themeColors['--color-error'] },
  pending: { color: themeColors['--color-warning'] },
  none: { color: themeColors['--color-foreground-alt'] },
  root: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    borderColor: themeColors['--color-ui-outline'],
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
    overflow: 'auto',
    backgroundColor: themeColors['--color-background-secondary'],
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  empty: { color: themeColors['--color-foreground-alt'] },
  label: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    display: 'flex',
  },
  itemLabel: {
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  detail: {
    color: themeColors['--color-foreground-alt'],
    flexShrink: 0,
    fontSize: '10px',
    lineHeight: 1.5,
  },
  row: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  button: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-background-tertiary'],
      },
    },
    cursor: 'pointer',
    display: 'flex',
    justifyContent: 'space-between',
    textAlign: 'left',
    width: '100%',
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
  className?: never
  // items is the array of items to display.
  items: StatusListItem[]
  // emptyMessage is shown when items is empty.
  emptyMessage?: ReactNode
  // statusLabels are optional custom labels for each status.
  statusLabels?: Partial<Record<StatusListItemStatus, string>>
  // sx overrides the container styles.
  sx?: StyleXStyles
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
      sx={[
        styles.indicator,
        status === 'success'
          ? styles.success
          : status === 'error'
            ? styles.error
            : status === 'pending'
              ? styles.pending
              : styles.none,
      ]}
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
  sx,
  onItemClick,
}: StatusListProps) {
  const labels = { ...defaultStatusLabels, ...statusLabels }
  return (
    <div sx={[styles.root, sx]}>
      {items.length === 0 ? (
        <div sx={styles.empty}>{emptyMessage}</div>
      ) : (
        items.map((item) => {
          const content = (
            <>
              <span sx={styles.label}>
                <StatusIndicator
                  status={item.status}
                  label={labels[item.status]}
                />
                <span sx={styles.itemLabel}>{item.label}</span>
              </span>
              {item.detail && <span sx={styles.detail}>{item.detail}</span>}
            </>
          )
          if (!onItemClick) {
            return (
              <div key={item.id} sx={styles.row}>
                {content}
              </div>
            )
          }
          return (
            <button
              key={item.id}
              type="button"
              sx={styles.button}
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
