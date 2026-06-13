import { describe, expect, it } from 'vitest'

import {
  getAllObjectViewers,
  getObjectViewersForType,
  getProductObjectViewers,
} from './viewers.js'

describe('viewer catalog split', () => {
  it('keeps base viewers out of the product catalog', () => {
    const productIDs = getProductObjectViewers().map(
      (viewer) => viewer.componentID,
    )

    expect(productIDs).not.toContain('spacewave.object-layout.viewer')
    expect(productIDs).not.toContain('spacewave.debug.viewer')
    expect(productIDs).toContain('spacewave.unixfs.viewer')
  })

  it('merges base, product, and downstream viewers in app order', () => {
    const downstreamViewer = {
      componentID: 'downstream.notes.viewer',
      typeID: 'downstream/notes',
      name: 'Downstream Notes',
      component: () => null,
    }

    const viewerIDs = getAllObjectViewers([downstreamViewer]).map(
      (viewer) => viewer.componentID,
    )

    expect(viewerIDs.slice(0, 2)).toEqual([
      'spacewave.object-layout.viewer',
      'spacewave.debug.viewer',
    ])
    expect(viewerIDs).toContain('spacewave.unixfs.viewer')
    expect(viewerIDs.at(-1)).toBe('downstream.notes.viewer')
  })

  it('finds downstream viewers by type', () => {
    const downstreamViewer = {
      componentID: 'downstream.notes.viewer',
      typeID: 'downstream/notes',
      name: 'Downstream Notes',
      component: () => null,
    }

    expect(
      getObjectViewersForType('downstream/notes', [downstreamViewer]),
    ).toContain(downstreamViewer)
  })
})
