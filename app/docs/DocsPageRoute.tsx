import { useMemo, useCallback } from 'react'
import { useParams, useNavigate } from '@s4wave/web/router/router.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'
import { DocsLayout } from './DocsLayout.js'
import { DocsSidebar } from './DocsSidebar.js'
import { DocsSearch } from './DocsSearch.js'
import { DocsPage } from './DocsPage.js'
import { loadDocs } from './load-docs.js'
import { getSections } from './sections.js'
import type { DocPage } from './types.js'

// DocsPageRoute reads route params and renders the matching documentation page.
export function DocsPageRoute() {
  const params = useParams()
  const navigate = useNavigate()
  const site = params['site']
  const section = params['section']
  const slug = params['slug']
  const url = `/docs/${site}/${section}/${slug}`

  const docs = useMemo(() => loadDocs(), [])
  const allSections = useMemo(() => getSections(), [])

  // Filter docs and sections to the current site for scoped navigation.
  const siteDocs = useMemo(
    () => docs.filter((d) => d.site === site),
    [docs, site],
  )
  const sections = useMemo(
    () => allSections.filter((s) => s.site === site),
    [allSections, site],
  )
  const docIndex = siteDocs.findIndex((d) => d.url === url)

  const handleSearchSelect = useCallback(
    (doc: DocPage) => {
      navigate({ path: doc.url })
    },
    [navigate],
  )

  if (docIndex === -1) {
    return <NavigatePath to="/docs" replace />
  }

  const doc = siteDocs[docIndex]
  const prevDoc = siteDocs[docIndex - 1]
  const nextDoc = siteDocs[docIndex + 1]

  const sidebar = (
    <div className="flex min-h-full flex-col gap-3">
      <div className="px-4 pt-4">
        <DocsSearch docs={siteDocs} onSelect={handleSearchSelect} />
      </div>
      <DocsSidebar sections={sections} currentSlug={slug} currentDoc={doc} />
    </div>
  )

  return (
    <DocsLayout sidebar={sidebar} currentSlug={slug}>
      <DocsPage doc={doc} prevDoc={prevDoc} nextDoc={nextDoc} />
    </DocsLayout>
  )
}
