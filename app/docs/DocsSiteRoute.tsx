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

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    minHeight: '100%',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

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
    <div sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
        <DocsSearch docs={docs} onSelect={handleSearchSelect} />
      </div>
      <DocsSidebar sections={sections} />
    </div>
  )

  return (
    <DocsLayout sidebar={sidebar} contentWidth="wide">
      <SiteHome siteId={site} sections={sections} />
    </DocsLayout>
  )
}
