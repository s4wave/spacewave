/* eslint-disable react-doctor/no-many-boolean-props */
import { useState, useRef, useEffect, useCallback, type ReactNode } from 'react'
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
    <PanelHeader ref={toolbarRef} className="gap-1.5" height={height}>
      {showNav && (
        <div className="flex items-center gap-0.5">
          <NavIconButton
            icon={<LuChevronLeft className="size-4" />}
            label="Back"
            onClick={onBack}
            disabled={!canGoBack}
          />
          <NavIconButton
            icon={<LuChevronRight className="size-4" />}
            label="Forward"
            onClick={onForward}
            disabled={!canGoForward}
          />
          <NavIconButton
            icon={<LuChevronUp className="size-4" />}
            label="Up"
            onClick={onUp}
            disabled={!canGoUp}
          />
        </div>
      )}

      {showPath ? (
        <PathBar
          path={currentPath}
          onPathChange={onPathChange}
          onNavigate={onNavigate}
        />
      ) : (
        <div className="flex-1" />
      )}

      {(onNewFolder || onUploadFiles) && (
        <div className="flex items-center gap-0.5">
          {onNewFolder && (
            <NavIconButton
              icon={<LuFolderPlus className="size-4" />}
              label="New folder"
              onClick={onNewFolder}
            />
          )}
          {onUploadFiles && (
            <NavIconButton
              icon={<LuUpload className="size-4" />}
              label="Upload files"
              onClick={onUploadFiles}
            />
          )}
        </div>
      )}

      {showOverflow ? (
        searchActive ? (
          <SearchBox
            placeholder="Search"
            focusOnMount
            onBlur={() => setSearchActive(false)}
          />
        ) : (
          <OverflowMenu
            collapseLevel={collapseLevel}
            onSearchClick={() => setSearchActive(true)}
            onBack={onBack}
            onForward={onForward}
            onUp={onUp}
            canGoBack={canGoBack}
            canGoForward={canGoForward}
            canGoUp={canGoUp}
          />
        )
      ) : (
        <SearchBox placeholder="Search" />
      )}
    </PanelHeader>
  )
}

interface NavIconButtonProps {
  icon: ReactNode
  label: string
  onClick?: () => void
  disabled?: boolean
}

// NavIconButton renders a compact toolbar action with foreground-alt to
// foreground hover, matching the design-system panel header convention.
function NavIconButton({ icon, label, onClick, disabled }: NavIconButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={label}
      aria-label={label}
      className={cn(
        'flex size-6 items-center justify-center rounded transition-colors',
        disabled
          ? 'text-foreground-alt/30 cursor-default'
          : 'text-foreground-alt hover:text-foreground hover:bg-foreground/5',
      )}
    >
      {icon}
    </button>
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
        <button
          type="button"
          aria-label="More actions"
          className="text-foreground-alt hover:text-foreground hover:bg-foreground/5 flex size-6 items-center justify-center rounded transition-colors"
        >
          <LuEllipsisVertical className="size-4" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="text-ui min-w-[140px]">
        <DropdownMenuItem onClick={onSearchClick}>
          <LuSearch className="size-3.5" />
          Search
        </DropdownMenuItem>
        {showNavItems && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={onBack} disabled={!canGoBack}>
              <LuChevronLeft className="size-3.5" />
              Back
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onForward} disabled={!canGoForward}>
              <LuChevronRight className="size-3.5" />
              Forward
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onUp} disabled={!canGoUp}>
              <LuChevronUp className="size-3.5" />
              Up
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
