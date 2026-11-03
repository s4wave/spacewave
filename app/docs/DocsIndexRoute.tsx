import { useMemo, useCallback } from 'react'
import { useNavigate } from '@s4wave/web/router/router.js'
import { DocsLayout } from './DocsLayout.js'
import { DocsSidebar } from './DocsSidebar.js'
import { DocsSearch } from './DocsSearch.js'
import { DocsHub } from './DocsHub.js'
import { getSections } from './sections.js'
import { loadDocs } from './load-docs.js'
import type { DocPage } from './types.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexDirection: 'column',
    minHeight: '100%',
  },
  rule2: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

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
    <div sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
        <DocsSearch docs={docs} onSelect={handleSearchSelect} />
      </div>
      <DocsSidebar sections={sections} />
    </div>
  )

  return (
    <DocsLayout sidebar={sidebar} contentWidth="wide">
      <DocsHub />
    </DocsLayout>
  )
}
