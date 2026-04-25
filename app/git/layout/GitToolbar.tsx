import { LuChevronLeft, LuChevronRight, LuChevronUp } from 'react-icons/lu'

import type { ListRefsResponse } from '@s4wave/sdk/git/repo.pb.js'

import { Toolbar } from '@s4wave/web/editors/file-browser/Toolbar.js'
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
}: GitToolbarProps) {
  return (
    <div className="bg-panel-header border-foreground/8 flex h-7 items-center border-b px-1">
      <RefSelector
        effectiveRef={effectiveRef}
        refsResponse={refsResponse}
        loading={refsLoading}
        onRefSelect={onRefSelect}
      />
      <div className="flex">
        <button
          onClick={onBack}
          disabled={!canGoBack}
          className={cn(
            'rounded p-[2px]',
            canGoBack ? 'hover:bg-pulldown-hover' : 'cursor-default opacity-40',
          )}
        >
          <LuChevronLeft className="text-foreground-alt h-4 w-4" />
        </button>
        <button
          onClick={onForward}
          disabled={!canGoForward}
          className={cn(
            'rounded p-[2px]',
            canGoForward ?
              'hover:bg-pulldown-hover'
            : 'cursor-default opacity-40',
          )}
        >
          <LuChevronRight className="text-foreground-alt h-4 w-4" />
        </button>
        <button
          onClick={onUp}
          disabled={currentPath === '/'}
          className={cn(
            'rounded p-[2px]',
            currentPath !== '/' ?
              'hover:bg-pulldown-hover'
            : 'cursor-default opacity-40',
          )}
        >
          <LuChevronUp className="text-foreground-alt h-4 w-4" />
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
    </div>
  )
}
