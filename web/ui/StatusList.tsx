import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { useEffect, useRef, type ReactNode } from 'react'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
} from '../style/tokens.stylex.js'

import {
  colors as themeColors2,
  utilityState as themeUtilityState,
  layout as themeLayout2,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-bold)',
    fontWeight: 'var(--font-weight-bold)',
    dynamicRangeLimit: 'no-limit',
    animationName: { default: null, ':is([data-status-changed])': 'hdr-flash' },
    animationDuration: { default: null, ':is([data-status-changed])': '.2s' },
    animationTimingFunction: {
      default: null,
      ':is([data-status-changed])': 'ease-out',
    },
    animationIterationCount: { default: null, ':is([data-status-changed])': 1 },
    animationDirection: {
      default: null,
      ':is([data-status-changed])': 'normal',
    },
    animationPlayState: {
      default: null,
      ':is([data-status-changed])': 'running',
    },
    animationDelay: { default: null, ':is([data-status-changed])': '0s' },
    animationFillMode: { default: null, ':is([data-status-changed])': 'none' },
    animationTimeline: { default: null, ':is([data-status-changed])': 'auto' },
  },
  rule2: { color: themeColors2['--color-success'] },
  rule3: { color: themeColors2['--color-error'] },
  rule4: { color: themeColors2['--color-warning'] },
  rule5: { color: themeColors2['--color-foreground-alt'] },
  rule6: {
    flex: 1,
    overflow: 'auto',
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors2['--color-ui-outline'],
    backgroundColor: themeColors2['--color-background-secondary'],
    padding: `calc(${themeLayout2['--spacing']} * 2)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors2['--color-foreground'],
  },
})

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
  className?: never
  // items is the array of items to display.
  items: StatusListItem[]
  // emptyMessage is shown when items is empty.
  emptyMessage?: ReactNode
  // statusLabels are optional custom labels for each status.
  statusLabels?: Partial<Record<StatusListItemStatus, string>>
  // className is an optional CSS class for the container.
  sx?: ComponentStyles
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
        stylexStyles2.rule1,
        status === 'success'
          ? stylexStyles2.rule2
          : status === 'error'
            ? stylexStyles2.rule3
            : status === 'pending'
              ? stylexStyles2.rule4
              : stylexStyles2.rule5,
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
}: Omit<StatusListProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  const labels = { ...defaultStatusLabels, ...statusLabels }
  return (
    <div sx={[stylexStyles2.rule6, sx]}>
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
