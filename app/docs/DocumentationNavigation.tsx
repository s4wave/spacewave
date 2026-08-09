import { useCallback, useMemo, useState, type ReactNode } from 'react'
import {
  LuBookOpen,
  LuFile,
  LuMenu,
  LuPlus,
  LuSearch,
  LuX,
} from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetTitle,
  SheetTrigger,
} from '@s4wave/web/ui/sheet.js'

interface DocumentationNavigationProps {
  title: string
  entries: readonly { name: string }[]
  selectedPage: string
  portalContainer: HTMLDivElement | null
  onSelectPage: (name: string) => void
  onCreatePage: () => Promise<void>
  children: ReactNode
}

// DocumentationNavigation owns page filtering and responsive navigation chrome.
export function DocumentationNavigation({
  title,
  entries,
  selectedPage,
  portalContainer,
  onSelectPage,
  onCreatePage,
  children,
}: DocumentationNavigationProps) {
  const [query, setQuery] = useState('')
  const [open, setOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<Error | null>(null)
  const filtered = useMemo(() => {
    if (!query) return entries
    const lower = query.toLowerCase()
    return entries.filter((entry) => entry.name.toLowerCase().includes(lower))
  }, [entries, query])

  const selectPage = useCallback(
    (name: string) => {
      setOpen(false)
      onSelectPage(name)
    },
    [onSelectPage],
  )
  const createPage = useCallback(async () => {
    if (creating) return
    setOpen(false)
    setCreating(true)
    setCreateError(null)
    try {
      await onCreatePage()
    } catch (error) {
      setCreateError(error instanceof Error ? error : new Error(String(error)))
    } finally {
      setCreating(false)
    }
  }, [creating, onCreatePage])

  const body = (
    <>
      <div className="border-border flex items-center gap-1 border-b px-2 py-1.5">
        <div className="bg-muted flex flex-1 items-center gap-1.5 rounded px-2 py-1">
          <LuSearch className="text-muted-foreground size-3 shrink-0" />
          <input
            type="text"
            aria-label="Search pages"
            placeholder="Search pages…"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            className="text-foreground placeholder:text-muted-foreground w-full border-none bg-transparent text-xs outline-none"
          />
        </div>
        <button
          type="button"
          className="text-foreground-alt hover:bg-list-hover-background hover:text-foreground flex items-center justify-center rounded p-1.5 disabled:opacity-50"
          onClick={() => void createPage()}
          disabled={creating}
          title="New page"
        >
          <LuPlus className="size-3.5" />
        </button>
      </div>
      {createError && (
        <div
          role="alert"
          className="border-destructive/30 text-destructive border-b px-3 py-2 text-xs"
        >
          Could not create page: {createError.message}
        </div>
      )}
      <div className="flex-1 overflow-y-auto">
        {filtered.length === 0 ? (
          <div className="text-muted-foreground flex flex-col items-center justify-center gap-3 p-6 text-center">
            {entries.length === 0 ? (
              <>
                <span className="text-xs">No pages yet</span>
                <button
                  type="button"
                  className="bg-brand text-brand-foreground rounded-md px-3 py-1.5 text-xs font-medium hover:opacity-90 disabled:opacity-50"
                  onClick={() => void createPage()}
                  disabled={creating}
                >
                  Create first page
                </button>
              </>
            ) : (
              <span className="text-xs">No matching pages</span>
            )}
          </div>
        ) : (
          filtered.map((entry) => (
            <button
              key={entry.name}
              type="button"
              className={cn(
                'flex w-full items-center gap-2 px-3 py-2 text-left text-xs hover:bg-list-hover-background',
                selectedPage === entry.name &&
                  'bg-list-active-selection-background text-list-active-selection-foreground',
              )}
              onClick={() => selectPage(entry.name)}
            >
              <LuFile className="size-3 shrink-0" />
              <span className="truncate">
                {entry.name.replace(/\.md$/, '')}
              </span>
            </button>
          ))
        )}
      </div>
    </>
  )

  return (
    <>
      <Sheet open={open} onOpenChange={setOpen}>
        <div className="border-foreground/8 flex h-9 shrink-0 items-center gap-2 border-b px-3 @lg:hidden">
          <SheetTrigger asChild>
            <button
              type="button"
              aria-label="Open documentation pages"
              className="text-foreground-alt hover:bg-list-hover-background hover:text-foreground -ml-1 flex items-center justify-center rounded p-1.5"
            >
              <LuMenu className="size-4" />
            </button>
          </SheetTrigger>
          <LuBookOpen className="text-foreground size-4 shrink-0" />
          <span className="text-foreground truncate text-sm font-semibold tracking-tight">
            {title}
          </span>
        </div>
        <SheetContent
          side="left"
          position="absolute"
          portalContainer={portalContainer}
          showCloseButton={false}
          className="w-[240px] max-w-[85%] gap-0 p-0"
        >
          <div className="flex min-h-0 flex-1 flex-col">
            <div className="border-foreground/8 flex items-center justify-between border-b px-3 py-2.5">
              <SheetTitle className="text-foreground flex items-center gap-2 text-sm font-semibold tracking-tight">
                <LuBookOpen className="size-4 shrink-0" />
                {title}
              </SheetTitle>
              <SheetClose asChild>
                <button
                  type="button"
                  aria-label="Close documentation pages"
                  className="text-foreground-alt hover:text-foreground rounded-md p-1.5 transition-colors"
                >
                  <LuX className="size-4" />
                </button>
              </SheetClose>
            </div>
            {body}
          </div>
        </SheetContent>
      </Sheet>
      <div className="flex min-h-0 flex-1 overflow-hidden">
        <div className="border-border hidden w-[220px] shrink-0 flex-col border-r @lg:flex">
          <div className="border-foreground/8 flex h-9 shrink-0 items-center gap-2 border-b px-3">
            <LuBookOpen className="text-foreground size-4 shrink-0" />
            <span className="text-foreground truncate text-sm font-semibold tracking-tight">
              {title}
            </span>
          </div>
          {body}
        </div>
        <div className="flex min-w-0 flex-1 flex-col">{children}</div>
      </div>
    </>
  )
}
