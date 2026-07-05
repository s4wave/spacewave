import { useState } from 'react'
import { LuMenu, LuX } from 'react-icons/lu'
import { cn } from '@s4wave/web/style/utils.js'
import { LegalFooter } from '@s4wave/app/landing/LegalFooter.js'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetTitle,
  SheetTrigger,
} from '@s4wave/web/ui/sheet.js'

// DocsLayoutProps defines the props for DocsLayout.
interface DocsLayoutProps {
  sidebar: React.ReactNode
  children: React.ReactNode
  currentSlug?: string
  // contentWidth selects the content column measure. 'reading' is the prose
  // measure for article pages; 'wide' is the card-grid measure so hub and
  // section index pages fill a desktop viewport instead of stranding a gutter.
  contentWidth?: 'reading' | 'wide'
}

interface DocsMobileNavProps {
  portalContainer: HTMLDivElement | null
  sidebar: React.ReactNode
}

// DocsMobileNav renders the full-width top bar with the hamburger trigger and
// the slide-in navigation drawer. It occupies its own row above the content so
// the trigger spans the viewport rather than sitting beside the content column.
function DocsMobileNav({ portalContainer, sidebar }: DocsMobileNavProps) {
  const [mobileOpen, setMobileOpen] = useState(false)

  return (
    <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
      <div className="border-b border-white/10 px-4 py-2.5 @lg:hidden">
        <SheetTrigger asChild>
          <button
            type="button"
            aria-label="Open documentation navigation"
            className="text-foreground-alt hover:text-foreground hover:border-foreground/20 inline-flex items-center gap-2 rounded-md border border-white/10 px-3 py-2 text-sm font-medium transition-colors"
          >
            <LuMenu className="size-4" />
            <span>Navigation</span>
          </button>
        </SheetTrigger>
      </div>
      <SheetContent
        side="left"
        position="absolute"
        portalContainer={portalContainer}
        showCloseButton={false}
        className="w-[264px] max-w-[85%] gap-0 p-0"
      >
        <div className="flex min-h-0 flex-1 flex-col">
          <div className="flex items-center justify-between border-b border-white/10 px-4 py-3">
            <SheetTitle className="text-sm font-semibold">
              Documentation
            </SheetTitle>
            <SheetClose asChild>
              <button
                type="button"
                aria-label="Close documentation navigation"
                className="text-foreground-alt hover:text-foreground rounded-md p-2 transition-colors"
              >
                <LuX className="size-4" />
              </button>
            </SheetClose>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto">{sidebar}</div>
        </div>
      </SheetContent>
    </Sheet>
  )
}

// DocsLayout renders the responsive documentation shell: a persistent left rail
// on wide displays that collapses into a top-bar drawer below the @lg container
// breakpoint. The content column carries its own min-w-0 so wide code blocks and
// tables scroll within it instead of forcing the page to scroll sideways.
export function DocsLayout({
  sidebar,
  children,
  currentSlug,
  contentWidth = 'reading',
}: DocsLayoutProps) {
  const [portalContainer, setPortalContainer] = useState<HTMLDivElement | null>(
    null,
  )
  const maxWidth = contentWidth === 'wide' ? 'max-w-5xl' : 'max-w-3xl'

  return (
    <div
      ref={setPortalContainer}
      className="bg-background-landing @container relative flex w-full flex-1 flex-col overflow-hidden"
    >
      {/* Navigation drawer - narrow widths only */}
      <DocsMobileNav
        key={currentSlug ?? 'docs-root'}
        portalContainer={portalContainer}
        sidebar={sidebar}
      />

      <div className="flex min-h-0 flex-1 overflow-hidden">
        {/* Sidebar - wide widths only */}
        <aside className="hidden w-[216px] shrink-0 overflow-y-auto border-r border-white/10 @lg:block">
          {sidebar}
        </aside>

        {/* Content. overflow-x-hidden keeps the scroll region from ever
            scrolling sideways: wide prose scrolls inside its own bounded box
            and the shell never gains a horizontal scrollbar. */}
        <main className="flex min-w-0 flex-1 flex-col overflow-x-hidden overflow-y-auto">
          <div
            className={cn(
              'mx-auto w-full flex-1 px-5 pt-6 pb-20 @lg:px-8 @lg:pt-10',
              maxWidth,
            )}
          >
            {children}
          </div>
          <LegalFooter />
        </main>
      </div>
    </div>
  )
}
