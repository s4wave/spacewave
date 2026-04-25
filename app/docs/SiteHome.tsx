import { useCallback, useMemo } from 'react'
import { LuArrowRight, LuBookOpen, LuArrowLeft } from 'react-icons/lu'
import { useNavigate } from '@s4wave/web/router/router.js'
import { siteDefs, type DocSite } from './sections.js'
import type { DocSection } from './types.js'

// SiteHomeProps defines the props for SiteHome.
interface SiteHomeProps {
  siteId: string
  sections: DocSection[]
}

// SiteHome renders the per-site documentation landing with section cards.
export function SiteHome({ siteId, sections }: SiteHomeProps) {
  const navigate = useNavigate()
  const site = useMemo(() => siteDefs.find((s) => s.id === siteId), [siteId])

  const goToPage = useCallback(
    (url: string) => {
      navigate({ path: url })
    },
    [navigate],
  )

  const goToHub = useCallback(() => {
    navigate({ path: '/docs' })
  }, [navigate])

  // Other sites for cross-links.
  const otherSites = useMemo(
    () => siteDefs.filter((s) => s.id !== siteId),
    [siteId],
  )

  return (
    <div>
      <header className="mb-10">
        <button
          onClick={goToHub}
          className="text-foreground-alt/50 hover:text-foreground-alt mb-4 flex cursor-pointer items-center gap-1.5 text-xs transition-colors"
        >
          <LuArrowLeft className="h-3 w-3" />
          All Documentation
        </button>
        <h1 className="text-foreground mb-3 text-2xl font-bold tracking-tight @lg:text-3xl">
          {site?.label ?? siteId}
        </h1>
        <p className="text-foreground-alt text-sm leading-relaxed @lg:text-base">
          {site?.description}
        </p>
      </header>

      <div className="grid gap-5 @lg:grid-cols-2">
        {sections.map((section) => {
          const firstPage = section.pages[0]
          return (
            <button
              key={section.id}
              onClick={() => firstPage && goToPage(firstPage.url)}
              disabled={!firstPage}
              className="border-foreground/6 hover:border-foreground/12 hover:bg-background-card/30 group flex cursor-pointer flex-col items-start gap-3 rounded-xl border p-6 text-left transition-all duration-200 disabled:cursor-default disabled:opacity-50"
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
            No pages yet. Check back soon.
          </p>
        </div>
      )}

      {otherSites.length > 0 && (
        <div className="mt-10 border-t border-white/10 pt-6">
          <h3 className="text-foreground-alt/50 mb-3 text-xs font-bold tracking-widest uppercase">
            Other Documentation
          </h3>
          <div className="flex flex-wrap gap-3">
            {otherSites.map((other) => (
              <button
                key={other.id}
                onClick={() => goToPage(`/docs/${other.id}`)}
                className="text-foreground-alt/70 hover:text-foreground cursor-pointer text-sm transition-colors"
              >
                {other.label}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
