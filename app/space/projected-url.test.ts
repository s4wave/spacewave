import { describe, expect, it } from 'vitest'
import {
  buildProjectedObjectContentPath,
  buildProjectedExportURL,
  buildProjectedFileInlineURL,
  buildProjectedFileURL,
} from './projected-url.js'

describe('projected app URLs', () => {
  it('builds raw, inline, and export URLs from the shared projected path owner', () => {
    const opts = {
      sessionIndex: 7,
      sharedObjectId: 'space-1',
      objectKey: 'docs/demo',
      path: '/nested/logo.png',
    }

    expect(buildProjectedFileURL(opts)).toBe(
      '/p/spacewave-core/fs/u/7/so/space-1/-/docs/demo/-/nested/logo.png',
    )
    expect(buildProjectedFileInlineURL(opts)).toBe(
      '/p/spacewave-core/fs/u/7/so/space-1/-/docs/demo/-/nested/logo.png?inline=1',
    )
    expect(buildProjectedExportURL(opts)).toBe(
      '/p/spacewave-core/export/u/7/so/space-1/-/docs/demo/-/nested/logo.png',
    )
  })

  it('marks projected object roots as content roots for export consumers', () => {
    const opts = {
      sessionIndex: 7,
      sharedObjectId: 'space-1',
      objectKey: 'docs/demo',
      path: '/',
    }

    expect(buildProjectedObjectContentPath(opts)).toBe(
      'u/7/so/space-1/-/docs/demo/-',
    )
    expect(buildProjectedExportURL(opts)).toBe(
      '/p/spacewave-core/export/u/7/so/space-1/-/docs/demo/-',
    )
    expect(buildProjectedFileURL(opts)).toBe(
      '/p/spacewave-core/fs/u/7/so/space-1/-/docs/demo/-',
    )
    expect(buildProjectedFileInlineURL(opts)).toBe(
      '/p/spacewave-core/fs/u/7/so/space-1/-/docs/demo/-?inline=1',
    )
  })
})
