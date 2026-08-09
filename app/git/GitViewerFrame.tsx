import type { ComponentProps, ReactNode, Ref } from 'react'

import { GitToolbar } from './layout/GitToolbar.js'
import { ViewModeSelector } from './layout/ViewModeSelector.js'
import type { ViewMode } from './layout/route.js'
import { SelectedRefBar } from './refs/SelectedRefBar.js'

export function GitViewerFrame({
  containerRef,
  toolbarProps,
  refBarProps,
  mode,
  onModeChange,
  hasReadme,
  availableModes,
  children,
}: {
  containerRef?: Ref<HTMLDivElement>
  toolbarProps: ComponentProps<typeof GitToolbar>
  refBarProps?: ComponentProps<typeof SelectedRefBar>
  mode?: ViewMode
  onModeChange?: (mode: ViewMode) => void
  hasReadme?: boolean
  availableModes?: ViewMode[]
  children: ReactNode
}) {
  return (
    <div
      ref={containerRef}
      className="flex h-full w-full flex-col overflow-hidden"
    >
      <GitToolbar {...toolbarProps} />
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
    </div>
  )
}
