import { useRef } from 'react'
import { LuChevronLeft, LuChevronRight, LuChevronUp } from 'react-icons/lu'

import type { ListRefsResponse } from '@s4wave/sdk/git/repo.pb.js'

import { Toolbar } from '@s4wave/web/editors/file-browser/Toolbar.js'
import {
  ViewerDetailsPress,
  type ViewerBarDetails,
} from '@s4wave/web/object/ViewerBar.js'
import { cn } from '@s4wave/web/style/utils.js'

import { RefSelector } from '../refs/RefSelector.js'

// GitToolbarProps are props for the GitToolbar component.
export interface GitToolbarProps {
  effectiveRef: string | null
  refsResponse: ListRefsResponse | null
  refsLoading: boolean
  onRefSelect: (refName: string) => void
  currentPath: string
  onPathChange: (path: string) => void
  onBack?: () => void
  onForward?: () => void
  onUp?: () => void
  canGoBack: boolean
  canGoForward: boolean
  showPath?: boolean
  // details puts a Details press at the toolbar's end that opens the
  // repository's facts.
  details?: ViewerBarDetails
}

// GitToolbar renders the toolbar with branch selector and path navigation.
export function GitToolbar({
  effectiveRef,
  refsResponse,
  refsLoading,
  onRefSelect,
  currentPath,
  onPathChange,
  onBack,
  onForward,
  onUp,
  canGoBack,
  canGoForward,
  showPath = true,
  details,
}: GitToolbarProps) {
  // Hold the toolbar, where a closing Details popover may send focus.
  const barRef = useRef<HTMLDivElement>(null)
  return (
    <div
      ref={barRef}
      tabIndex={-1}
      className="bg-panel-header border-foreground/8 focus-ring flex h-7 items-center border-b px-1 pointer-coarse:h-auto"
    >
      <RefSelector
        effectiveRef={effectiveRef}
        refsResponse={refsResponse}
        loading={refsLoading}
        onRefSelect={onRefSelect}
      />
      <div className="flex">
        <button
          type="button"
          aria-label="Back"
          onClick={onBack}
          disabled={!canGoBack}
          className={cn(
            'flex size-6 items-center justify-center rounded pointer-coarse:size-11',
            canGoBack ? 'hover:bg-pulldown-hover' : 'cursor-default opacity-40',
          )}
        >
          <LuChevronLeft aria-hidden className="text-foreground-alt size-4" />
        </button>
        <button
          type="button"
          aria-label="Forward"
          onClick={onForward}
          disabled={!canGoForward}
          className={cn(
            'flex size-6 items-center justify-center rounded pointer-coarse:size-11',
            canGoForward
              ? 'hover:bg-pulldown-hover'
              : 'cursor-default opacity-40',
          )}
        >
          <LuChevronRight aria-hidden className="text-foreground-alt size-4" />
        </button>
        <button
          type="button"
          aria-label="Up one directory"
          onClick={onUp}
          disabled={currentPath === '/'}
          className={cn(
            'flex size-6 items-center justify-center rounded pointer-coarse:size-11',
            currentPath !== '/'
              ? 'hover:bg-pulldown-hover'
              : 'cursor-default opacity-40',
          )}
        >
          <LuChevronUp aria-hidden className="text-foreground-alt size-4" />
        </button>
      </div>
      {showPath && (
        <div className="min-w-0 flex-1">
          <Toolbar
            currentPath={currentPath}
            onPathChange={onPathChange}
            onNavigate={onPathChange}
            height={28}
            hideNav
          />
        </div>
      )}
      {details ? (
        <div className="ms-auto flex shrink-0 items-center">
          <ViewerDetailsPress barRef={barRef} {...details} />
        </div>
      ) : null}
    </div>
  )
}
