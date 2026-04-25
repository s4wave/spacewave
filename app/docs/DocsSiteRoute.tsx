import { useMemo, useCallback } from 'react'
import { useParams, useNavigate } from '@s4wave/web/router/router.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'
import { DocsLayout } from './DocsLayout.js'
import { DocsSidebar } from './DocsSidebar.js'
import { DocsSearch } from './DocsSearch.js'
import { SiteHome } from './SiteHome.js'
import { loadDocs } from './load-docs.js'
import { getSections, siteDefs } from './sections.js'
import type { DocPage } from './types.js'

// DocsSiteRoute renders the site-level docs landing for a given audience surface.
export function DocsSiteRoute() {
  const params = useParams()
  const navigate = useNavigate()
  const site = params['site']

  const docs = useMemo(() => loadDocs(), [])
  const allSections = useMemo(() => getSections(), [])
  const sections = useMemo(
    () => allSections.filter((s) => s.site === site),
    [allSections, site],
  )
  const siteDocs = useMemo(
    () => docs.filter((d) => d.site === site),
    [docs, site],
  )

  const validSite = useMemo(() => siteDefs.some((s) => s.id === site), [site])

  const handleSearchSelect = useCallback(
    (doc: DocPage) => {
      navigate({ path: doc.url })
    },
    [navigate],
  )

  if (!validSite) {
    return <NavigatePath to="/docs" replace />
  }

  const sidebar = (
    <div className="flex min-h-full flex-col gap-3">
      <div className="px-4 pt-4">
        <DocsSearch docs={siteDocs} onSelect={handleSearchSelect} />
      </div>
      <DocsSidebar sections={sections} />
    </div>
  )

  return (
    <DocsLayout sidebar={sidebar}>
      <SiteHome siteId={site} sections={sections} />
    </DocsLayout>
  )
}
