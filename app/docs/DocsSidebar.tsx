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
    <nav className="flex flex-1 flex-col">
      <div className="flex flex-col gap-1 p-4">
        <button
          onClick={goToIndex}
          className={cn(
            'mb-4 cursor-pointer text-left text-sm font-semibold transition-colors',
            !currentDoc
              ? 'text-brand'
              : 'text-foreground hover:text-foreground-alt',
          )}
        >
          Documentation
        </button>

        {categories.map((cat) => (
          <div key={cat.name} className="mb-5">
            <h2 className="text-foreground-alt/40 mb-2 text-[10px] font-semibold tracking-widest uppercase">
              {cat.name}
            </h2>
            {cat.sections.map((section) => (
              <div key={`${section.site}/${section.id}`} className="mb-3">
                <h3 className="text-foreground-alt/50 mb-1.5 text-xs font-semibold tracking-wider uppercase">
                  {section.label}
                </h3>
                <ul className="flex flex-col">
                  {section.pages.map((page) => (
                    <li key={page.url}>
                      <button
                        onClick={() => goToPage(page.url)}
                        className={cn(
                          'w-full cursor-pointer border-l-2 py-2 pl-3 text-left text-sm transition-colors',
                          currentDoc?.url === page.url
                            ? 'border-brand text-brand'
                            : 'text-foreground-alt/70 hover:text-foreground hover:border-foreground-alt/20 border-transparent',
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
          className="text-foreground-alt/50 hover:text-foreground-alt flex items-center gap-1.5 text-xs transition-colors"
        >
          <LuExternalLink className="size-3" />
          View on GitHub
        </ExternalLink>
        <button
          onClick={currentDoc ? goToIndex : goHome}
          className="text-foreground-alt/50 hover:text-foreground-alt flex cursor-pointer items-center gap-1.5 text-left text-xs transition-colors"
        >
          <LuArrowLeft className="size-3" />
          {currentDoc ? 'Back to Documentation' : 'Back to Home'}
        </button>
      </div>
    </nav>
  )
}
