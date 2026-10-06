/* eslint-disable react-doctor/no-many-boolean-props */
import {
  useState,
  useRef,
  useEffect,
  useCallback,
  type DragEvent,
  type ReactNode,
} from 'react'
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
  if (width < 480) return 'nav'
  if (width < 600) return 'menus'
  return 'none'
}

type PathTargetDragOver = (
  path: string,
  event: DragEvent<HTMLElement>,
) => boolean
type PathTargetDrop = (path: string, event: DragEvent<HTMLElement>) => void

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
  upDropPath?: string
  onNewFolder?: () => void
  onUploadFiles?: () => void
  onPathTargetDragOver?: PathTargetDragOver
  onPathTargetDrop?: PathTargetDrop
  height?: number
  hideNav?: boolean
}

// upDropHandlers makes a control accept drops onto the parent folder, when
// that folder exists.
function upDropHandlers(
  canGoUp: boolean,
  upDropPath: string | undefined,
  onPathTargetDragOver: PathTargetDragOver | undefined,
  onPathTargetDrop: PathTargetDrop | undefined,
) {
  if (!canGoUp || !upDropPath) return {}

  return {
    onDragOver: (event: DragEvent<HTMLElement>) =>
      onPathTargetDragOver?.(upDropPath, event),
    onDrop: (event: DragEvent<HTMLElement>) =>
      onPathTargetDrop?.(upDropPath, event),
  }
}

// useCollapseLevel tracks how far the toolbar must collapse for its width.
function useCollapseLevel() {
  const [collapseLevel, setCollapseLevel] = useState<CollapseLevel>('none')
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

  return { collapseLevel, toolbarRef }
}

// ToolbarNav renders the back, forward, and up buttons.
function ToolbarNav({
  onBack,
  onForward,
  onUp,
  canGoBack,
  canGoForward,
  canGoUp,
  upDrop,
}: {
  onBack?: () => void
  onForward?: () => void
  onUp?: () => void
  canGoBack: boolean
  canGoForward: boolean
  canGoUp: boolean
  upDrop: ReturnType<typeof upDropHandlers>
}) {
  return (
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
        {...upDrop}
      />
    </div>
  )
}

// ToolbarActions renders the new folder and upload buttons.
function ToolbarActions({
  showNewFolder,
  onNewFolder,
  onUploadFiles,
}: {
  showNewFolder: boolean
  onNewFolder?: () => void
  onUploadFiles?: () => void
}) {
  if (!showNewFolder && !onUploadFiles) return null

  return (
    <div className="flex items-center gap-0.5">
      {showNewFolder && (
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
  )
}

const searchBoxClassName =
  '[@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11'

/** Toolbar keeps the file path and primary action visible as its width shrinks. */
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
  upDropPath,
  onNewFolder,
  onUploadFiles,
  onPathTargetDragOver,
  onPathTargetDrop,
  height,
  hideNav,
}: ToolbarProps) {
  const { collapseLevel, toolbarRef } = useCollapseLevel()
  const [searchActive, setSearchActive] = useState(false)

  // Keep upload visible while navigation and folder creation move into More.
  const wide = collapseLevel === 'none' || collapseLevel === 'menus'
  const showNewFolder = !!onNewFolder && (!onUploadFiles || wide)

  let search = <SearchBox placeholder="Search" className={searchBoxClassName} />
  if (collapseLevel !== 'none') {
    search = searchActive ? (
      <SearchBox
        placeholder="Search"
        focusOnMount
        onBlur={() => setSearchActive(false)}
        className={searchBoxClassName}
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
        upDropPath={upDropPath}
        onPathTargetDragOver={onPathTargetDragOver}
        onPathTargetDrop={onPathTargetDrop}
        onNewFolder={onNewFolder}
      />
    )
  }

  return (
    <PanelHeader
      ref={toolbarRef}
      variant="compact"
      height={height}
      className="[@media(pointer:coarse)]:min-h-11"
    >
      {!hideNav && wide && (
        <ToolbarNav
          onBack={onBack}
          onForward={onForward}
          onUp={onUp}
          canGoBack={canGoBack}
          canGoForward={canGoForward}
          canGoUp={canGoUp}
          upDrop={upDropHandlers(
            canGoUp,
            upDropPath,
            onPathTargetDragOver,
            onPathTargetDrop,
          )}
        />
      )}

      {collapseLevel !== 'path' && !searchActive ? (
        <PathBar
          path={currentPath}
          onPathChange={onPathChange}
          onNavigate={onNavigate}
          onPathTargetDragOver={onPathTargetDragOver}
          onPathTargetDrop={onPathTargetDrop}
        />
      ) : (
        <div className="flex-1" />
      )}

      <ToolbarActions
        showNewFolder={showNewFolder}
        onNewFolder={onNewFolder}
        onUploadFiles={onUploadFiles}
      />

      {search}
    </PanelHeader>
  )
}

interface NavIconButtonProps {
  icon: ReactNode
  label: string
  onClick?: () => void
  onDragOver?: (event: DragEvent<HTMLButtonElement>) => void
  onDrop?: (event: DragEvent<HTMLButtonElement>) => void
  disabled?: boolean
}

// NavIconButton renders a compact toolbar action with foreground-alt to
// foreground hover, matching the design-system panel header convention.
function NavIconButton({
  icon,
  label,
  onClick,
  onDragOver,
  onDrop,
  disabled,
}: NavIconButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      onDragOver={onDragOver}
      onDrop={onDrop}
      title={label}
      aria-label={label}
      className={cn(
        'flex size-6 items-center justify-center rounded transition-colors [@media(pointer:coarse)]:size-11',
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
  upDropPath?: string
  onPathTargetDragOver?: PathTargetDragOver
  onPathTargetDrop?: PathTargetDrop
  onNewFolder?: () => void
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
  upDropPath,
  onPathTargetDragOver,
  onPathTargetDrop,
  onNewFolder,
}: OverflowMenuProps) {
  const showNavItems = collapseLevel === 'nav' || collapseLevel === 'path'
  const showNewFolderItem = showNavItems && onNewFolder

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label="More actions"
          className="text-foreground-alt hover:text-foreground hover:bg-foreground/5 flex size-6 items-center justify-center rounded transition-colors [@media(pointer:coarse)]:size-11"
        >
          <LuEllipsisVertical className="size-4" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" variant="compact" className="min-w-35">
        <DropdownMenuItem
          className="max-sm:min-h-12 [@media(pointer:coarse)]:min-h-12"
          onClick={onSearchClick}
        >
          <LuSearch className="size-3.5" />
          Search
        </DropdownMenuItem>
        {showNavItems && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              className="max-sm:min-h-12 [@media(pointer:coarse)]:min-h-12"
              onClick={onBack}
              disabled={!canGoBack}
            >
              <LuChevronLeft className="size-3.5" />
              Back
            </DropdownMenuItem>
            <DropdownMenuItem
              className="max-sm:min-h-12 [@media(pointer:coarse)]:min-h-12"
              onClick={onForward}
              disabled={!canGoForward}
            >
              <LuChevronRight className="size-3.5" />
              Forward
            </DropdownMenuItem>
            <DropdownMenuItem
              className="max-sm:min-h-12 [@media(pointer:coarse)]:min-h-12"
              onClick={onUp}
              disabled={!canGoUp}
              {...upDropHandlers(
                canGoUp,
                upDropPath,
                onPathTargetDragOver,
                onPathTargetDrop,
              )}
            >
              <LuChevronUp className="size-3.5" />
              Up
            </DropdownMenuItem>
          </>
        )}
        {showNewFolderItem && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              className="max-sm:min-h-12 [@media(pointer:coarse)]:min-h-12"
              onClick={onNewFolder}
            >
              <LuFolderPlus className="size-3.5" />
              New folder
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
