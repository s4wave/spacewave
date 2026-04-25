import { useRef, useState, useEffect, useCallback } from 'react'
import { cn } from '@s4wave/web/style/utils.js'
import { LuChevronLeft, LuChevronRight, LuX } from 'react-icons/lu'
import { AppLogo } from '@s4wave/web/images/AppLogo.js'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'

interface TopBarProps {
  activeWorkspace: string
  workspaces: Array<{ id: string; name: string }>
  onWorkspaceChange: (id: string) => void
  onWorkspaceClose?: (id: string) => void
  onWorkspaceAdd?: () => void
}

const menuItems = ['File', 'Edit', 'View', 'Tools', 'Help']

export function TopBar({
  activeWorkspace,
  workspaces,
  onWorkspaceChange,
  onWorkspaceClose,
  onWorkspaceAdd,
}: TopBarProps) {
  const scrollContainerRef = useRef<HTMLDivElement>(null)
  const menuContainerRef = useRef<HTMLDivElement>(null)
  const topBarRef = useRef<HTMLDivElement>(null)
  const isScrollingRef = useRef(false)
  const [scrollState, setScrollState] = useState({
    canScrollLeft: false,
    canScrollRight: false,
  })
  const [hideMenuItems, setHideMenuItems] = useState(false)

  const checkScroll = useCallback(() => {
    if (!scrollContainerRef.current || isScrollingRef.current) return

    const { scrollLeft, scrollWidth, clientWidth } = scrollContainerRef.current
    setScrollState({
      canScrollLeft: scrollLeft > 0,
      canScrollRight: scrollLeft < scrollWidth - clientWidth - 1,
    })
  }, [])

  const scroll = useCallback((direction: 'left' | 'right') => {
    if (!scrollContainerRef.current) return

    const scrollAmount = 150
    const { scrollLeft, scrollWidth, clientWidth } = scrollContainerRef.current

    const targetScrollLeft =
      direction === 'left' ?
        Math.max(0, scrollLeft - scrollAmount)
      : Math.min(scrollWidth - clientWidth, scrollLeft + scrollAmount)

    isScrollingRef.current = true

    setScrollState({
      canScrollLeft: targetScrollLeft > 0,
      canScrollRight: targetScrollLeft < scrollWidth - clientWidth - 1,
    })

    scrollContainerRef.current.scrollTo({
      left: targetScrollLeft,
      behavior: 'smooth',
    })
  }, [])

  const checkMenuFit = useCallback(() => {
    const topBar = topBarRef.current

    if (!topBar) return

    const totalBarWidth = topBar.clientWidth

    const logoWidth = 30
    const menuItemsWidth = menuItems.length * 55
    const minTabWidth = 30
    const navButtonWidth = 28
    const addButtonWidth = 36
    const padding = 32
    const minRequiredTabSpace =
      minTabWidth + navButtonWidth * 2 + addButtonWidth + padding

    const minRequiredWidth = logoWidth + menuItemsWidth + minRequiredTabSpace

    const shouldHide = totalBarWidth < minRequiredWidth

    setHideMenuItems(shouldHide)
  }, [])

  useEffect(() => {
    checkScroll()
    checkMenuFit()

    const container = scrollContainerRef.current
    const topBar = topBarRef.current
    if (!container || !topBar) return

    const handleScrollEnd = () => {
      isScrollingRef.current = false
      checkScroll()
    }

    const scrollObserver = new ResizeObserver(checkScroll)
    scrollObserver.observe(container)

    const topBarObserver = new ResizeObserver(checkMenuFit)
    topBarObserver.observe(topBar)

    container.addEventListener('scroll', checkScroll)
    container.addEventListener('scrollend', handleScrollEnd)

    return () => {
      scrollObserver.disconnect()
      topBarObserver.disconnect()
      container.removeEventListener('scroll', checkScroll)
      container.removeEventListener('scrollend', handleScrollEnd)
    }
  }, [workspaces, checkScroll, checkMenuFit])

  const { canScrollLeft, canScrollRight } = scrollState

  return (
    <div
      ref={topBarRef}
      className="bg-topbar-back flex h-[var(--spacing-shell-header)] items-center overflow-hidden leading-tight"
    >
      <div
        ref={menuContainerRef}
        className="flex h-full shrink-0 items-center gap-px pr-2 pl-1.5"
      >
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              aria-label="Open app menu"
              className="flex items-center justify-center"
              title="Open app menu"
            >
              <AppLogo className="h-[28px] w-[28px]" />
            </button>
          </DropdownMenuTrigger>
          {hideMenuItems && (
            <DropdownMenuContent align="start">
              {menuItems.map((menu) => (
                <DropdownMenuItem key={menu}>{menu}</DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          )}
        </DropdownMenu>
        <div
          className={cn(
            'flex h-full items-center gap-px overflow-hidden transition-all duration-200 select-none',
            hideMenuItems ? 'w-0 opacity-0' : 'w-auto opacity-100',
          )}
        >
          {menuItems.map((menu) => (
            <button
              key={menu}
              className="rounded-menu-button text-topbar-button-text hover:text-topbar-button-text-hi hover:bg-pulldown-hover text-topbar-menu text-shadow-glow flex h-5 items-center justify-center px-[7px] whitespace-nowrap transition-colors"
            >
              {menu}
            </button>
          ))}
        </div>
      </div>

      {/* Workspace Tabs */}
      <div className="flex h-full min-w-0 flex-1 items-end px-2 pb-px select-none">
        {canScrollLeft && (
          <button
            onClick={() => scroll('left')}
            className="bg-shell-tab-inactive hover:bg-shell-tab-active/50 text-shell-tab-text border-foreground/8 mr-0.5 mb-px flex h-5 shrink-0 items-center justify-center rounded-t-lg border border-b-0 px-1 transition-colors"
            title="Scroll left"
          >
            <LuChevronLeft className="h-3 w-3" />
          </button>
        )}

        <div
          ref={scrollContainerRef}
          className="hide-scrollbar flex h-full min-w-0 flex-1 items-end gap-0.5 overflow-x-auto overflow-y-hidden"
        >
          {workspaces.map((workspace) => (
            <div
              key={workspace.id}
              className={cn(
                'group relative flex h-5 shrink-0 items-center transition-colors',
                'border-foreground/8 border border-b-0',
                'rounded-t-lg',
                'max-w-[120px] min-w-[30px]',
                activeWorkspace === workspace.id ?
                  'bg-shell-tab-active text-shell-tab-text-active'
                : 'bg-shell-tab-inactive text-shell-tab-text hover:bg-shell-tab-active/50',
              )}
              style={
                activeWorkspace === workspace.id ?
                  { boxShadow: 'inset 0 -1px 0 var(--color-widget-emboss)' }
                : undefined
              }
            >
              <button
                onClick={() => onWorkspaceChange(workspace.id)}
                className="h-full flex-1 overflow-hidden px-2 pt-0 pb-0.5 text-left tracking-tight text-ellipsis whitespace-nowrap"
                title={workspace.name}
              >
                {workspace.name}
              </button>
              {onWorkspaceClose && (
                <button
                  onClick={(e) => {
                    e.stopPropagation()
                    onWorkspaceClose(workspace.id)
                  }}
                  className="text-shell-tab-text hover:text-shell-tab-text-active mr-1 flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded-sm opacity-0 transition-opacity group-hover:opacity-100"
                  title="Close tab"
                >
                  <LuX className="h-2.5 w-2.5" />
                </button>
              )}
            </div>
          ))}

          {!(canScrollLeft || canScrollRight) && onWorkspaceAdd && (
            <button
              onClick={onWorkspaceAdd}
              className={cn(
                'h-5 shrink-0 px-2 pb-[1.5px] transition-colors',
                'border-foreground/8 border border-b-0',
                'rounded-t-lg',
                'bg-shell-tab-inactive text-shell-tab-text hover:bg-shell-tab-active/50',
                'flex items-center justify-center',
              )}
              title="Add workspace"
            >
              +
            </button>
          )}
        </div>

        {canScrollRight && (
          <button
            onClick={() => scroll('right')}
            className="bg-shell-tab-inactive hover:bg-shell-tab-active/50 text-shell-tab-text border-foreground/8 mb-px ml-0.5 flex h-5 shrink-0 items-center justify-center rounded-t-lg border border-b-0 px-1 transition-colors"
            title="Scroll right"
          >
            <LuChevronRight className="h-3 w-3" />
          </button>
        )}

        {(canScrollLeft || canScrollRight) && onWorkspaceAdd && (
          <button
            onClick={onWorkspaceAdd}
            className={cn(
              'h-5 shrink-0 px-2 pb-[1.5px] transition-colors',
              'border-foreground/8 border border-b-0',
              'rounded-t-lg',
              'bg-shell-tab-inactive text-shell-tab-text hover:bg-shell-tab-active/50',
              'mb-px ml-0.5 flex items-center justify-center',
            )}
            title="Add workspace"
          >
            +
          </button>
        )}
      </div>
    </div>
  )
}
