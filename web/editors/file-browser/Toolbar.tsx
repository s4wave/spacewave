import { useState, useRef, useEffect, useCallback } from 'react'
import {
  LuChevronLeft,
  LuChevronRight,
  LuChevronUp,
  LuEllipsisVertical,
  LuFolderPlus,
  LuSearch,
  LuUpload,
} from 'react-icons/lu'
import { PanelHeader } from '../../ui/PanelHeader.js'
import { PathBar } from './PathBar.js'
import { SearchBox } from '../../ui/SearchBox.js'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '../../ui/DropdownMenu.js'
import { cn } from '../../style/utils.js'

type CollapseLevel = 'none' | 'menus' | 'nav' | 'path'

function getCollapseLevel(width: number): CollapseLevel {
  if (width < 180) return 'path'
  if (width < 260) return 'nav'
  if (width < 420) return 'menus'
  return 'none'
}

interface ToolbarProps {
  currentPath: string
  onPathChange?: (path: string) => void
  onNavigate?: (path: string) => void
  onBack?: () => void
  onForward?: () => void
  onUp?: () => void
  canGoBack?: boolean
  canGoForward?: boolean
  canGoUp?: boolean
  onNewFolder?: () => void
  onUploadFiles?: () => void
  height?: number
  hideNav?: boolean
}

export function Toolbar({
  currentPath,
  onPathChange,
  onNavigate,
  onBack,
  onForward,
  onUp,
  canGoBack = false,
  canGoForward = false,
  canGoUp = true,
  onNewFolder,
  onUploadFiles,
  height,
  hideNav,
}: ToolbarProps) {
  const [collapseLevel, setCollapseLevel] = useState<CollapseLevel>('none')
  const [searchActive, setSearchActive] = useState(false)
  const toolbarRef = useRef<HTMLDivElement>(null)

  const checkWidth = useCallback(() => {
    if (!toolbarRef.current) return
    setCollapseLevel(getCollapseLevel(toolbarRef.current.clientWidth))
  }, [])

  useEffect(() => {
    checkWidth()
    const toolbar = toolbarRef.current
    if (!toolbar) return

    const observer = new ResizeObserver(checkWidth)
    observer.observe(toolbar)
    return () => observer.disconnect()
  }, [checkWidth])

  const showNav =
    !hideNav && (collapseLevel === 'none' || collapseLevel === 'menus')
  const showPath = collapseLevel !== 'path'
  const showOverflow = collapseLevel !== 'none'

  return (
    <PanelHeader
      ref={toolbarRef}
      className="bg-panel-header gap-1"
      height={height}
    >
      {showNav && (
        <div className="flex">
          <button
            onClick={onBack}
            disabled={!canGoBack}
            title="Back"
            aria-label="Back"
            className={cn(
              'rounded p-[2px]',
              canGoBack ?
                'hover:bg-pulldown-hover'
              : 'cursor-default opacity-40',
            )}
          >
            <LuChevronLeft className="text-foreground-alt h-4 w-4" />
          </button>
          <button
            onClick={onForward}
            disabled={!canGoForward}
            title="Forward"
            aria-label="Forward"
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
            disabled={!canGoUp}
            title="Up"
            aria-label="Up"
            className={cn(
              'rounded p-[2px]',
              canGoUp ? 'hover:bg-pulldown-hover' : 'cursor-default opacity-40',
            )}
          >
            <LuChevronUp className="text-foreground-alt h-4 w-4" />
          </button>
        </div>
      )}

      {showPath ?
        <PathBar
          path={currentPath}
          onPathChange={onPathChange}
          onNavigate={onNavigate}
        />
      : <div className="flex-1" />}

      {(onNewFolder || onUploadFiles) && (
        <div className="flex gap-1">
          {onNewFolder && (
            <button
              onClick={onNewFolder}
              title="New folder"
              className="hover:bg-pulldown-hover rounded p-[2px]"
            >
              <LuFolderPlus className="text-foreground-alt h-4 w-4" />
            </button>
          )}
          {onUploadFiles && (
            <button
              onClick={onUploadFiles}
              title="Upload files"
              className="hover:bg-pulldown-hover rounded p-[2px]"
            >
              <LuUpload className="text-foreground-alt h-4 w-4" />
            </button>
          )}
        </div>
      )}

      {showOverflow ?
        searchActive ?
          <SearchBox
            placeholder="Search"
            autoFocus
            onBlur={() => setSearchActive(false)}
          />
        : <OverflowMenu
            collapseLevel={collapseLevel}
            onSearchClick={() => setSearchActive(true)}
            onBack={onBack}
            onForward={onForward}
            onUp={onUp}
            canGoBack={canGoBack}
            canGoForward={canGoForward}
            canGoUp={canGoUp}
          />

      : <SearchBox placeholder="Search" />}
    </PanelHeader>
  )
}

interface OverflowMenuProps {
  collapseLevel: CollapseLevel
  onSearchClick: () => void
  onBack?: () => void
  onForward?: () => void
  onUp?: () => void
  canGoBack?: boolean
  canGoForward?: boolean
  canGoUp?: boolean
}

function OverflowMenu({
  collapseLevel,
  onSearchClick,
  onBack,
  onForward,
  onUp,
  canGoBack = false,
  canGoForward = false,
  canGoUp = true,
}: OverflowMenuProps) {
  const showNavItems = collapseLevel === 'nav' || collapseLevel === 'path'

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button className="hover:bg-pulldown-hover rounded p-[2px]">
          <LuEllipsisVertical className="text-foreground-alt h-4 w-4" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="text-ui min-w-[140px]">
        <DropdownMenuItem onClick={onSearchClick}>
          <LuSearch className="h-3.5 w-3.5" />
          Search
        </DropdownMenuItem>
        {showNavItems && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={onBack} disabled={!canGoBack}>
              <LuChevronLeft className="h-3.5 w-3.5" />
              Back
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onForward} disabled={!canGoForward}>
              <LuChevronRight className="h-3.5 w-3.5" />
              Forward
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onUp} disabled={!canGoUp}>
              <LuChevronUp className="h-3.5 w-3.5" />
              Up
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
