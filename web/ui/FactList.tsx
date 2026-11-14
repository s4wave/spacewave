import type { ReactNode } from 'react'

import { CopyButton } from '@s4wave/web/ui/CopyButton.js'
import { MiddleEllipsis } from '@s4wave/web/ui/MiddleEllipsis.js'
import { cn } from '@s4wave/web/style/utils.js'

// FactListProps configures a FactList.
export interface FactListProps {
  children?: ReactNode
  // dense packs the rows at viewer-row, 24px or 44px under a coarse pointer,
  // for a side column or popover; a list in the body keeps viewer-fact-row.
  // A row's copy press is 24px either way, so it meets the target floor.
  dense?: boolean
  // className is appended to the list's class list.
  className?: string
}

// FactList holds FactRows and is the inline-size container facts they read,
// so each row lays out by the list's own width, not the pane's or window's:
// below 13rem a row's label sits over its value. A dense list sets --fact-row,
// the height of each row's line, which is viewer-fact-row unset.
export function FactList({ children, dense, className }: FactListProps) {
  return (
    <div
      className={cn(
        '@container/facts flex flex-col',
        dense && '[--fact-row:var(--spacing-viewer-row)]',
        className,
      )}
    >
      {children}
    </div>
  )
}

// FactRowProps configures a FactRow.
export interface FactRowProps {
  // label names the fact.
  label: string
  // value is the fact as text, shown on one line, with its whole text in its
  // title and on copy.
  value: string
  // kind says how the value gives way when it does not fit: an id (a hash,
  // key, peer ID, path or URL) is mono and shortened in its middle; text
  // ends in an ellipsis. Defaults to id.
  kind?: 'id' | 'text'
  // sub is a second fact, one muted line under the value, such as a
  // commit's subject under its hash.
  sub?: string
  // control is the press that acts on the value. Defaults to a copy press,
  // or none for an empty value; null leaves the control column empty. A
  // placeholder for a missing value, such as Unknown, is a text fact with
  // control null, never a copyable id.
  control?: ReactNode
}

// labelWidth is the label column: viewer-fact-label, widening to
// viewer-fact-label-roomy from 28rem of the list's own width, so a list in a
// side column or popover keeps the narrow label in a wide pane.
const labelWidth =
  '[--fact-label:var(--spacing-viewer-fact-label)] @min-[28rem]/facts:[--fact-label:var(--spacing-viewer-fact-label-roomy)]'

// stacked moves a cell to the layout under 13rem of the list's width, where
// the label sits over the value and the control stays beside the value.
const stacked = {
  row: '@max-[13rem]/facts:grid-cols-[minmax(0,1fr)_auto] @max-[13rem]/facts:pt-1',
  label:
    '@max-[13rem]/facts:col-start-1 @max-[13rem]/facts:row-start-1 @max-[13rem]/facts:h-4',
  value: '@max-[13rem]/facts:col-start-1 @max-[13rem]/facts:row-start-2',
  control: '@max-[13rem]/facts:col-start-2 @max-[13rem]/facts:row-start-2',
  sub: '@max-[13rem]/facts:col-start-1 @max-[13rem]/facts:row-start-3',
}

// FactRow is one fact as one line of label, value and control,
// viewer-fact-row tall, or viewer-row in a dense FactList. Nothing in it
// wraps: a value that does not fit is shortened, an id in its middle and text
// at its end, so one-fact rows share one height and two-fact rows another,
// and every control sits on one right edge. The data-fact, data-fact-value
// and data-fact-sub attributes mark the parts for the viewer clip check.
export function FactRow({
  label,
  value,
  kind = 'id',
  sub,
  control,
}: FactRowProps) {
  const id = kind === 'id'
  // An empty value has nothing to copy, so it gets no press.
  const press =
    control !== undefined ? (
      control
    ) : value ? (
      <CopyButton text={value} label={`Copy ${label}`} />
    ) : null
  return (
    <div
      data-fact
      className={cn(
        'grid grid-cols-[var(--fact-label)_minmax(0,1fr)_auto] gap-x-2 text-xs whitespace-nowrap',
        labelWidth,
        stacked.row,
      )}
    >
      <span
        className={cn(
          'text-foreground-alt h-[var(--fact-row,var(--spacing-viewer-fact-row))] col-start-1 row-start-1 flex min-w-0 items-center select-none',
          stacked.label,
        )}
      >
        <span className="min-w-0 truncate" title={label}>
          {label}
        </span>
      </span>
      <span
        data-fact-value={kind}
        className={cn(
          'text-foreground h-[var(--fact-row,var(--spacing-viewer-fact-row))] col-start-2 row-start-1 flex min-w-0 items-center overflow-hidden',
          stacked.value,
        )}
      >
        {id ? (
          <MiddleEllipsis value={value} />
        ) : (
          <span className="min-w-0 truncate" title={value}>
            {value}
          </span>
        )}
      </span>
      <span
        className={cn(
          'h-[var(--fact-row,var(--spacing-viewer-fact-row))] col-start-3 row-start-1 flex items-center',
          stacked.control,
        )}
      >
        {press}
      </span>
      {sub && (
        <span
          data-fact-sub
          className={cn(
            'text-foreground-alt col-span-2 col-start-2 row-start-2 min-w-0 truncate pb-1 leading-4',
            stacked.sub,
          )}
          title={sub}
        >
          {sub}
        </span>
      )}
    </div>
  )
}
