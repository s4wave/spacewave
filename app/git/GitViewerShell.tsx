import type { ComponentProps, ReactNode, Ref } from 'react'

import {
  ViewerSide,
  type ViewerSideState,
} from '@s4wave/web/object/ViewerSide.js'

import { GitToolbar } from './layout/GitToolbar.js'
import { ViewModeSelector } from './layout/ViewModeSelector.js'
import type { ViewMode } from './layout/route.js'
import { SelectedRefBar } from './refs/SelectedRefBar.js'

export function GitViewerCenteredState({
  containerRef,
  title,
  subtitle,
  detail,
  action,
}: {
  containerRef?: Ref<HTMLDivElement>
  title: ReactNode
  subtitle?: ReactNode
  detail?: ReactNode
  action?: ReactNode
}) {
  return (
    <div
      ref={containerRef}
      className="flex h-full w-full flex-col items-center justify-center overflow-hidden"
    >
      <div className="text-foreground text-sm font-semibold">{title}</div>
      {subtitle && (
        <div className="text-foreground-alt mt-1 text-xs font-medium">
          {subtitle}
        </div>
      )}
      {detail && (
        <div className="text-foreground-alt/70 mt-2 text-xs">{detail}</div>
      )}
      {action}
    </div>
  )
}

// GitViewerFrameProps configures a GitViewerFrame.
interface GitViewerFrameProps {
  containerRef?: Ref<HTMLDivElement>
  toolbarProps: ComponentProps<typeof GitToolbar>
  refBarProps?: ComponentProps<typeof SelectedRefBar>
  mode?: ViewMode
  onModeChange?: (mode: ViewMode) => void
  hasReadme?: boolean
  availableModes?: ViewMode[]
  // facts are the repository's FactRows. The toolbar's Details press opens
  // them wherever the frame has no side column.
  facts?: ReactNode
  // side shows the facts in a Repository side column from a 42rem pane,
  // starting open or as a rail, with Details only below 42rem.
  side?: ViewerSideState
  // onSideChange hears the column's Collapse and Expand, so the caller can
  // keep its state across a view without the column.
  onSideChange?: (side: ViewerSideState) => void
  // detailsOpen opens the Details popover on mount, for a fixture that
  // shows it.
  detailsOpen?: boolean
  children: ReactNode
}

/**
 * GitViewerFrame lays out a Git viewer: the toolbar, the selected ref's bar,
 * the view selector and the body. With facts, the toolbar carries Details,
 * and with side the ref bar, views and body sit beside a Repository side
 * column, so the facts show or are one press away at every pane width.
 */
export function GitViewerFrame({
  containerRef,
  toolbarProps,
  refBarProps,
  mode,
  onModeChange,
  hasReadme,
  availableModes,
  facts,
  side,
  onSideChange,
  detailsOpen,
  children,
}: GitViewerFrameProps) {
  // Draw the ref bar, the view selector and the body as one stack.
  const body = (
    <>
      {refBarProps && <SelectedRefBar {...refBarProps} />}
      {mode && onModeChange && (
        <ViewModeSelector
          mode={mode}
          onModeChange={onModeChange}
          hasReadme={!!hasReadme}
          availableModes={availableModes}
        />
      )}
      {children}
    </>
  )
  const sided = !!facts && !!side
  const details = facts
    ? { label: 'Repository', facts, narrow: sided, defaultOpen: detailsOpen }
    : undefined

  return (
    <div
      ref={containerRef}
      className="flex h-full w-full flex-col overflow-hidden"
    >
      <GitToolbar {...toolbarProps} details={details} />
      {sided ? (
        <ViewerSide
          label="Repository"
          facts={facts}
          initial={side}
          onChange={onSideChange}
        >
          {body}
        </ViewerSide>
      ) : (
        body
      )}
    </div>
  )
}
