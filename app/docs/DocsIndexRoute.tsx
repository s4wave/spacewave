import { useMemo, useCallback } from 'react'
import { useNavigate } from '@s4wave/web/router/router.js'
import { DocsLayout } from './DocsLayout.js'
import { DocsSidebar } from './DocsSidebar.js'
import { DocsSearch } from './DocsSearch.js'
import { DocsHub } from './DocsHub.js'
import { getSections } from './sections.js'
import { loadDocs } from './load-docs.js'
import type { DocPage } from './types.js'

// DocsIndexRoute renders the docs hub page with audience surface cards.
export function DocsIndexRoute() {
  const sections = useMemo(() => getSections(), [])
  const docs = useMemo(() => loadDocs(), [])
  const navigate = useNavigate()

  const handleSearchSelect = useCallback(
    (doc: DocPage) => {
      navigate({ path: doc.url })
    },
    [navigate],
  )

  const sidebar = (
    <div className="flex min-h-full flex-col gap-3">
      <div className="px-4 pt-4">
        <DocsSearch docs={docs} onSelect={handleSearchSelect} />
      </div>
      <DocsSidebar sections={sections} />
    </div>
  )

  return (
    <DocsLayout sidebar={sidebar}>
      <DocsHub />
    </DocsLayout>
  )
}
