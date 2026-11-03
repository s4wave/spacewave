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
} from '@s4wave/web/images/icons.js'
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

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../style/tokens.stylex.js'

import {
  layout as themeLayout2,
  motion as themeMotion,
  colors as themeColors,
} from '../../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout2['--spacing']} * .5)`,
  },
  rule2: {
    width: `calc(${themeLayout2['--spacing']} * 4)`,
    height: `calc(${themeLayout2['--spacing']} * 4)`,
  },
  rule3: { flex: 1 },
  rule4: {
    display: 'flex',
    width: {
      default: `calc(${themeLayout2['--spacing']} * 6)`,
      '@media (pointer: coarse)': `calc(${themeLayout2['--spacing']} * 11)`,
    },
    height: {
      default: `calc(${themeLayout2['--spacing']} * 6)`,
      '@media (pointer: coarse)': `calc(${themeLayout2['--spacing']} * 11)`,
    },
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule5: {
    cursor: 'default',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
  },
  rule6: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule7: {
    display: 'flex',
    width: {
      default: `calc(${themeLayout2['--spacing']} * 6)`,
      '@media (pointer: coarse)': `calc(${themeLayout2['--spacing']} * 11)`,
    },
    height: {
      default: `calc(${themeLayout2['--spacing']} * 6)`,
      '@media (pointer: coarse)': `calc(${themeLayout2['--spacing']} * 11)`,
    },
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule8: { minWidth: `calc(${themeLayout2['--spacing']} * 35)` },
  rule9: {
    minHeight: {
      default: null,
      '@media not all and (min-width: 40rem)': `calc(${themeLayout2['--spacing']} * 12)`,
      '@media (pointer: coarse)': `calc(${themeLayout2['--spacing']} * 12)`,
    },
  },
  rule10: {
    width: `calc(${themeLayout2['--spacing']} * 3.5)`,
    height: `calc(${themeLayout2['--spacing']} * 3.5)`,
  },
})

const stylexStyles = stylex.create({
  header: {
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  rule1: {
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    minWidth: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
})

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
  // Keep the toolbar's collapse level tied to its mounted width.
  const [collapseLevel, setCollapseLevel] = useState<CollapseLevel>('none')
  const toolbarRef = useRef<HTMLDivElement>(null)

  // Resolve the available controls from the toolbar's current width.
  const checkWidth = useCallback(() => {
    if (!toolbarRef.current) return
    setCollapseLevel(getCollapseLevel(toolbarRef.current.clientWidth))
  }, [])

  // Measure the toolbar on mount and watch subsequent width changes.
  useEffect(() => {
    // Require a mounted toolbar before attaching its resize observer.
    checkWidth()
    const toolbar = toolbarRef.current
    if (!toolbar) return

    // Disconnect the width observer when this toolbar is replaced.
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
    <div sx={stylexStyles2.rule1}>
      <NavIconButton
        icon={<LuChevronLeft sx={stylexStyles2.rule2} />}
        label="Back"
        onClick={onBack}
        disabled={!canGoBack}
      />
      <NavIconButton
        icon={<LuChevronRight sx={stylexStyles2.rule2} />}
        label="Forward"
        onClick={onForward}
        disabled={!canGoForward}
      />
      <NavIconButton
        icon={<LuChevronUp sx={stylexStyles2.rule2} />}
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
    <div sx={stylexStyles2.rule1}>
      {showNewFolder && (
        <NavIconButton
          icon={<LuFolderPlus sx={stylexStyles2.rule2} />}
          label="New folder"
          onClick={onNewFolder}
        />
      )}
      {onUploadFiles && (
        <NavIconButton
          icon={<LuUpload sx={stylexStyles2.rule2} />}
          label="Upload files"
          onClick={onUploadFiles}
        />
      )}
    </div>
  )
}

/** Toolbar keeps the file path and primary action visible as its width shrinks. */
export function Toolbar({
  sx,
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
}: ToolbarProps & { sx?: stylex.StyleXStyles; className?: never }) {
  // Keep compact search activation beside the measured toolbar layout.
  const { collapseLevel, toolbarRef } = useCollapseLevel()
  const [searchActive, setSearchActive] = useState(false)

  // Keep upload visible while navigation and folder creation move into More.
  const wide = collapseLevel === 'none' || collapseLevel === 'menus'
  const showNewFolder = !!onNewFolder && (!onUploadFiles || wide)

  // Choose the expanded search field or compact search trigger.
  let search = <SearchBox placeholder="Search" sx={stylexStyles.rule1} />
  if (collapseLevel !== 'none') {
    search = searchActive ? (
      <SearchBox
        placeholder="Search"
        focusOnMount
        onBlur={() => setSearchActive(false)}
        sx={stylexStyles.rule1}
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
      sx={[stylexStyles.header, sx]}
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
        <div sx={stylexStyles2.rule3} />
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
      sx={[
        stylexStyles2.rule4,
        disabled ? stylexStyles2.rule5 : stylexStyles2.rule6,
      ]}
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
          sx={stylexStyles2.rule7}
        >
          <LuEllipsisVertical sx={stylexStyles2.rule2} />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        variant="compact"
        sx={stylexStyles2.rule8}
      >
        <DropdownMenuItem sx={stylexStyles2.rule9} onClick={onSearchClick}>
          <LuSearch sx={stylexStyles2.rule10} />
          Search
        </DropdownMenuItem>
        {showNavItems && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              sx={stylexStyles2.rule9}
              onClick={onBack}
              disabled={!canGoBack}
            >
              <LuChevronLeft sx={stylexStyles2.rule10} />
              Back
            </DropdownMenuItem>
            <DropdownMenuItem
              sx={stylexStyles2.rule9}
              onClick={onForward}
              disabled={!canGoForward}
            >
              <LuChevronRight sx={stylexStyles2.rule10} />
              Forward
            </DropdownMenuItem>
            <DropdownMenuItem
              sx={stylexStyles2.rule9}
              onClick={onUp}
              disabled={!canGoUp}
              {...upDropHandlers(
                canGoUp,
                upDropPath,
                onPathTargetDragOver,
                onPathTargetDrop,
              )}
            >
              <LuChevronUp sx={stylexStyles2.rule10} />
              Up
            </DropdownMenuItem>
          </>
        )}
        {showNewFolderItem && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem sx={stylexStyles2.rule9} onClick={onNewFolder}>
              <LuFolderPlus sx={stylexStyles2.rule10} />
              New folder
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
