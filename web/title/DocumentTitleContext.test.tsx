import { cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import {
  buildDocumentTitle,
  DocumentTitleProvider,
  getRouteDocumentTitleParts,
  getSpaceDocumentTitleName,
  useDocumentTitle,
} from './DocumentTitleContext.js'

function TitleProbe({
  view,
  space,
  active = true,
  priority = 0,
}: {
  view?: string
  space?: string
  active?: boolean
  priority?: number
}) {
  useDocumentTitle({ view, space }, { active, priority })
  return null
}

afterEach(() => cleanup())

beforeEach(() => {
  document.title = 'Initial title'
})

describe('buildDocumentTitle', () => {
  it('formats the most-specific context and removes empty duplicates', () => {
    expect(buildDocumentTitle({ view: 'Notes', space: 'Research' })).toBe(
      'Notes - Research - Spacewave',
    )
    expect(buildDocumentTitle({ view: ' Spacewave ', space: '' })).toBe(
      'Spacewave',
    )
  })

  it('hides opaque identifiers for unnamed Spaces', () => {
    expect(getSpaceDocumentTitleName('Research', 'space-1')).toBe('Research')
    expect(getSpaceDocumentTitleName('  ', 'space-1')).toBe('Space')
    expect(getSpaceDocumentTitleName('space-1', 'space-1')).toBe('Space')
  })
  it('derives concise route fallbacks without replacing the landing brand', () => {
    expect(getRouteDocumentTitleParts('/', 'Home')).toEqual({})
    expect(getRouteDocumentTitleParts('/landing', 'Tab')).toEqual({})
    expect(getRouteDocumentTitleParts('/', 'Research')).toEqual({
      view: 'Research',
    })
    expect(getRouteDocumentTitleParts('/pricing', 'Tab')).toEqual({
      view: 'Pricing',
    })
    expect(getRouteDocumentTitleParts('/landing/drive', 'Tab')).toEqual({
      view: 'Drive',
    })
    expect(getRouteDocumentTitleParts('/docs/reference', 'Docs')).toEqual({
      view: 'Docs',
    })
    expect(getRouteDocumentTitleParts('/%E0%A4%A', 'Tab')).toEqual({
      view: '%E0%A4%A',
    })
  })
})

describe('DocumentTitleProvider', () => {
  it('updates from the highest active context and restores the fallback', async () => {
    const view = render(
      <DocumentTitleProvider>
        <TitleProbe view="Docs" />
        <TitleProbe view="Notes" space="Research" priority={20} />
        <TitleProbe view="Hidden" priority={30} active={false} />
      </DocumentTitleProvider>,
    )

    await waitFor(() =>
      expect(document.title).toBe('Notes - Research - Spacewave'),
    )

    view.rerender(
      <DocumentTitleProvider>
        <TitleProbe view="Docs" />
        <TitleProbe view="Notes" space="Lab" priority={20} active={false} />
      </DocumentTitleProvider>,
    )
    await waitFor(() => expect(document.title).toBe('Docs - Spacewave'))

    view.rerender(<DocumentTitleProvider>{null}</DocumentTitleProvider>)
    await waitFor(() => expect(document.title).toBe('Spacewave'))
  })
})
