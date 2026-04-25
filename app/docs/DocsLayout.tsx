import { useState } from 'react'
import { LuMenu, LuX } from 'react-icons/lu'
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
}

interface DocsMobileSidebarProps {
  portalContainer: HTMLDivElement | null
  sidebar: React.ReactNode
}

// DocsMobileSidebar renders the mobile hamburger and drawer.
function DocsMobileSidebar({
  portalContainer,
  sidebar,
}: DocsMobileSidebarProps) {
  const [mobileOpen, setMobileOpen] = useState(false)

  return (
    <div className="@lg:hidden">
      <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
        <div className="border-b border-white/10 px-4 py-3">
          <SheetTrigger asChild>
            <button
              type="button"
              aria-label="Open documentation navigation"
              className="text-foreground-alt hover:text-foreground inline-flex items-center gap-2 rounded-md border border-white/10 px-3 py-2 text-sm font-medium transition-colors"
            >
              <LuMenu className="h-4 w-4" />
              <span>Navigation</span>
            </button>
          </SheetTrigger>
        </div>
        <SheetContent
          side="left"
          position="absolute"
          portalContainer={portalContainer}
          showCloseButton={false}
          className="w-[216px] gap-0 p-0"
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
                  <LuX className="h-4 w-4" />
                </button>
              </SheetClose>
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto">{sidebar}</div>
          </div>
        </SheetContent>
      </Sheet>
    </div>
  )
}

// DocsLayout renders the two-column layout for documentation pages.
export function DocsLayout({
  sidebar,
  children,
  currentSlug,
}: DocsLayoutProps) {
  const [portalContainer, setPortalContainer] = useState<HTMLDivElement | null>(
    null,
  )

  return (
    <div
      ref={setPortalContainer}
      className="bg-background-landing @container relative flex w-full flex-1 flex-col overflow-hidden"
    >
      <div className="flex flex-1 overflow-hidden">
        {/* Sidebar - desktop */}
        <aside className="hidden w-[216px] shrink-0 overflow-y-auto border-r border-white/10 @lg:block">
          {sidebar}
        </aside>

        {/* Sidebar - mobile drawer */}
        <DocsMobileSidebar
          key={currentSlug ?? 'docs-root'}
          portalContainer={portalContainer}
          sidebar={sidebar}
        />

        {/* Content */}
        <main className="flex flex-1 flex-col overflow-y-auto">
          <div className="mx-auto w-full max-w-4xl flex-1 px-4 pt-4 pb-20 @lg:px-8 @lg:pt-10">
            {children}
          </div>
          <LegalFooter />
        </main>
      </div>
    </div>
  )
}
