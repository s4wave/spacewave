import { describe, expect, it } from 'vitest'

import { getV86CatalogErrorCopy } from './VmV86WizardViewer.js'

describe('getV86CatalogErrorCopy', () => {
  it('translates an unpublished catalog error without exposing backend detail', () => {
    const raw =
      'build cdn world engine: cdn shared object has no published head'

    const copy = getV86CatalogErrorCopy(new Error(raw))

    expect(copy).toEqual({
      title: 'No VM images are published yet',
      detail: 'This image catalog has no published images to copy.',
      unpublished: true,
    })
    expect(`${copy.title} ${copy.detail}`).not.toContain(raw)
  })

  it('gives other catalog failures a retryable user-level presentation', () => {
    const copy = getV86CatalogErrorCopy(new Error('dial tcp: connection reset'))

    expect(copy).toEqual({
      title: 'Image catalog unavailable',
      detail: 'The VM image catalog could not be loaded. Try again.',
      unpublished: false,
    })
  })
})
