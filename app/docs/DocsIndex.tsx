import { useCallback } from 'react'
import { LuArrowRight, LuBookOpen } from 'react-icons/lu'
import { useNavigate } from '@s4wave/web/router/router.js'
import type { DocSection } from './types.js'

// DocsIndexProps defines the props for DocsIndex.
export interface DocsIndexProps {
  sections: DocSection[]
}

// DocsIndex renders the documentation landing page with section cards.
export function DocsIndex({ sections }: DocsIndexProps) {
  const navigate = useNavigate()

  const goToSection = useCallback(
    (url: string) => {
      navigate({ path: url })
    },
    [navigate],
  )

  return (
    <div>
      {/* Hero header */}
      <header className="mb-10">
        <h1 className="text-foreground mb-3 text-2xl font-bold tracking-tight @lg:text-3xl">
          Documentation
        </h1>
        <p className="text-foreground-alt text-sm leading-relaxed @lg:text-base">
          Learn how to build and manage your spaces with Spacewave.
        </p>
      </header>

      {/* Section cards */}
      <div className="grid gap-5 @lg:grid-cols-2">
        {sections.map((section) => {
          const firstPage = section.pages[0]
          return (
            <button
              key={section.id}
              onClick={() => firstPage && goToSection(firstPage.url)}
              disabled={!firstPage}
              className="border-foreground/6 hover:border-foreground/12 hover:bg-background-card/30 group flex cursor-pointer flex-col items-start gap-3 rounded-xl border p-6 text-left transition-all duration-200"
            >
              <div className="bg-brand/10 text-brand flex h-10 w-10 items-center justify-center rounded-lg">
                <LuBookOpen className="h-5 w-5" />
              </div>
              <h2 className="text-foreground text-lg font-semibold">
                {section.label}
              </h2>
              <p className="text-foreground-alt/70 text-sm leading-relaxed">
                {section.pages.length}{' '}
                {section.pages.length === 1 ? 'page' : 'pages'}
              </p>
              {firstPage && (
                <span className="text-brand group-hover:text-brand-highlight mt-auto flex items-center gap-1.5 text-xs font-medium transition-colors">
                  Get started
                  <LuArrowRight className="h-3 w-3 transition-transform duration-200 group-hover:translate-x-0.5" />
                </span>
              )}
            </button>
          )
        })}
      </div>

      {sections.every((s) => s.pages.length === 0) && (
        <div className="border-foreground/6 rounded-xl border border-dashed px-8 py-20 text-center">
          <p className="text-foreground-alt text-sm">
            No documentation pages yet. Check back soon.
          </p>
        </div>
      )}
    </div>
  )
}
