import { useCallback, useMemo } from 'react'
import { LuArrowLeft, LuExternalLink } from 'react-icons/lu'
import { GITHUB_REPO_URL } from '@s4wave/app/github.js'
import { ExternalLink } from '@s4wave/app/landing/ExternalLink.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'
import { siteDefs } from './sections.js'
import type { DocSection, DocPage } from './types.js'

// DocsSidebarProps defines the props for DocsSidebar.
interface DocsSidebarProps {
  sections: DocSection[]
  currentDoc?: DocPage
}

// DocsSidebar renders the navigation sidebar showing all sections and pages.
export function DocsSidebar({ sections, currentDoc }: DocsSidebarProps) {
  const navigate = useNavigate()

  const goToPage = useCallback(
    (url: string) => {
      navigate({ path: url })
    },
    [navigate],
  )

  const goToIndex = useCallback(() => {
    navigate({ path: '/docs' })
  }, [navigate])

  const goHome = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  const githubUrl = currentDoc
    ? `${GITHUB_REPO_URL}/blob/master/app/docs/content/${currentDoc.site}/${currentDoc.section}/${currentDoc.filename}`
    : `${GITHUB_REPO_URL}/tree/master/app/docs/content`

  // Group sections by site, preserving order.
  const siteLabels = useMemo(
    () => new Map(siteDefs.map((s) => [s.id, s.label])),
    [],
  )
  const categories = useMemo(() => {
    const cats: { name: string; sections: DocSection[] }[] = []
    for (const section of sections) {
      const label = siteLabels.get(section.site) ?? section.site
      const last = cats[cats.length - 1]
      if (last && last.name === label) {
        last.sections.push(section)
      } else {
        cats.push({ name: label, sections: [section] })
      }
    }
    return cats
  }, [sections, siteLabels])

  return (
    <nav aria-label="Documentation navigation" className="flex flex-1 flex-col">
      <div className="flex flex-col px-3 py-4">
        <button
          type="button"
          onClick={goToIndex}
          aria-current={!currentDoc ? 'page' : undefined}
          className={cn(
            'mb-4 min-h-11 cursor-pointer @2xl:min-h-9 rounded-md px-2 text-left text-xs font-semibold transition-colors focus-visible:ring-1 focus-visible:ring-brand/30 focus-visible:outline-none',
            !currentDoc
              ? 'text-brand'
              : 'text-foreground hover:text-foreground-alt',
          )}
        >
          Documentation
        </button>

        {categories.map((cat) => (
          <div key={cat.name} className="mb-4">
            <h2 className="text-foreground-alt/50 mb-1.5 px-2 text-xs font-semibold tracking-[0.08em] uppercase">
              {cat.name}
            </h2>
            {cat.sections.map((section) => (
              <div key={`${section.site}/${section.id}`} className="mb-2.5">
                <h3 className="text-foreground-alt/60 mb-1 px-2 text-xs font-medium tracking-[0.08em] uppercase">
                  {section.label}
                </h3>
                <ul className="flex flex-col">
                  {section.pages.map((page) => (
                    <li key={page.url}>
                      <button
                        type="button"
                        onClick={() => goToPage(page.url)}
                        aria-current={
                          currentDoc?.url === page.url ? 'page' : undefined
                        }
                        className={cn(
                          'min-h-11 w-full cursor-pointer @2xl:min-h-9 rounded-r-md border-l-2 px-2 py-1.5 text-left text-xs leading-4 transition-colors focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-brand/30 focus-visible:outline-none',
                          currentDoc?.url === page.url
                            ? 'border-brand bg-brand/8 text-brand'
                            : 'text-foreground-alt/70 hover:bg-foreground/5 hover:text-foreground hover:border-foreground-alt/20 border-transparent',
                        )}
                      >
                        {page.title}
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        ))}
      </div>

      <div className="mt-auto flex flex-col gap-1.5 border-t border-white/10 p-4">
        <ExternalLink
          href={githubUrl}
          className="text-foreground-alt/50 hover:bg-foreground/5 hover:text-foreground-alt focus-visible:ring-brand/30 flex min-h-11 items-center gap-1.5 rounded-md px-2 text-xs transition-colors focus-visible:ring-1 focus-visible:outline-none @2xl:min-h-9"
        >
          <LuExternalLink className="size-3" />
          View on GitHub
        </ExternalLink>
        <button
          onClick={currentDoc ? goToIndex : goHome}
          className="text-foreground-alt/50 hover:bg-foreground/5 hover:text-foreground-alt focus-visible:ring-brand/30 flex min-h-11 cursor-pointer items-center gap-1.5 rounded-md px-2 text-left text-xs transition-colors focus-visible:ring-1 focus-visible:outline-none @2xl:min-h-9"
        >
          <LuArrowLeft className="size-3" />
          {currentDoc ? 'Back to Documentation' : 'Back to Home'}
        </button>
      </div>
    </nav>
  )
}
