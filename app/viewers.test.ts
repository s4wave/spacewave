import { describe, expect, it } from 'vitest'

import { getObjectViewersForType } from './viewers.js'

describe('getObjectViewersForType', () => {
  it('keeps the UnixFS browser ahead of the gallery in default order', () => {
    const viewers = getObjectViewersForType('unixfs/fs-node')

    expect(viewers.slice(0, 2).map((viewer) => viewer.name)).toEqual([
      'UnixFS Viewer',
      'UnixFS Gallery',
    ])
  })
})
