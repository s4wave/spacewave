import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import {
  buildUnixFSBatchExportURL,
  buildUnixFSFileInlineURL,
  buildUnixFSFileDownloadURL,
  buildUnixFSSelectionDownloadDragTarget,
  downloadUnixFSSelection,
} from './download.js'

const h = vi.hoisted(() => ({
  mockDownloadURL: vi.fn(),
}))

vi.mock('@s4wave/web/download.js', () => ({
  downloadURL: h.mockDownloadURL,
}))

function buildEntry(id: string, name: string, isDir: boolean): FileEntry {
  return { id, name, isDir }
}

describe('unixfs download dispatch', () => {
  beforeEach(() => {
    h.mockDownloadURL.mockReset()
  })

  it('builds raw projected file urls', () => {
    expect(
      buildUnixFSFileDownloadURL(
        7,
        'space-1',
        'docs/demo',
        '/nested/hello.txt',
      ),
    ).toBe('/p/spacewave-core/fs/u/7/so/space-1/-/docs/demo/-/nested/hello.txt')
  })

  it('builds inline raw projected file urls', () => {
    expect(
      buildUnixFSFileInlineURL(7, 'space-1', 'docs/demo', '/nested/logo.png'),
    ).toBe(
      '/p/spacewave-core/fs/u/7/so/space-1/-/docs/demo/-/nested/logo.png?inline=1',
    )
  })

  it('uses raw file serving for one selected file', async () => {
    await downloadUnixFSSelection({
      sessionIndex: 3,
      sharedObjectId: 'space-a',
      objectKey: 'docs/demo',
      currentPath: '/nested',
      entries: [buildEntry('file', 'hello.txt', false)],
    })

    expect(h.mockDownloadURL).toHaveBeenCalledWith(
      '/p/spacewave-core/fs/u/3/so/space-a/-/docs/demo/-/nested/hello.txt',
      'hello.txt',
    )
  })

  it('uses subtree export for one selected directory', async () => {
    await downloadUnixFSSelection({
      sessionIndex: 3,
      sharedObjectId: 'space-a',
      objectKey: 'docs/demo',
      currentPath: '/',
      entries: [buildEntry('docs', 'assets', true)],
    })

    expect(h.mockDownloadURL).toHaveBeenCalledWith(
      '/p/spacewave-core/export/u/3/so/space-a/-/docs/demo/-/assets',
      'assets.zip',
    )
  })

  it('uses batch export for multi-selection', async () => {
    await downloadUnixFSSelection({
      sessionIndex: 5,
      sharedObjectId: 'space-b',
      objectKey: 'docs/demo',
      currentPath: '/nested',
      entries: [
        buildEntry('b', 'beta.txt', false),
        buildEntry('a', 'alpha', true),
      ],
    })

    expect(h.mockDownloadURL).toHaveBeenCalledTimes(1)
    const firstCall = h.mockDownloadURL.mock.calls[0] as [string, string]
    expect(firstCall[0]).toMatch(
      /^\/p\/spacewave-core\/export-batch\/u\/5\/so\/space-b\/-\/docs\/demo\/-\/nested\/.+\/selection\.zip$/,
    )
    expect(firstCall[1]).toBe('selection.zip')
  })

  it('builds batch export urls synchronously', () => {
    const batch = buildUnixFSBatchExportURL(
      5,
      'space-b',
      'docs/demo',
      '/nested',
      [buildEntry('b', 'beta.txt', false), buildEntry('a', 'alpha', true)],
    )

    expect(batch.url).toMatch(
      /^\/p\/spacewave-core\/export-batch\/u\/5\/so\/space-b\/-\/docs\/demo\/-\/nested\/.+\/selection\.zip$/,
    )
    expect(batch.filename).toBe('selection.zip')
  })

  it('builds download drag targets for file, directory, and selection', () => {
    expect(
      buildUnixFSSelectionDownloadDragTarget({
        sessionIndex: 3,
        sharedObjectId: 'space-a',
        objectKey: 'docs/demo',
        currentPath: '/nested',
        entries: [buildEntry('file', 'hello.txt', false)],
      }),
    ).toEqual({
      mimeType: 'application/octet-stream',
      filename: 'hello.txt',
      url: '/p/spacewave-core/fs/u/3/so/space-a/-/docs/demo/-/nested/hello.txt',
    })

    expect(
      buildUnixFSSelectionDownloadDragTarget({
        sessionIndex: 3,
        sharedObjectId: 'space-a',
        objectKey: 'docs/demo',
        currentPath: '/',
        entries: [buildEntry('docs', 'assets', true)],
      }),
    ).toEqual({
      mimeType: 'application/zip',
      filename: 'assets.zip',
      url: '/p/spacewave-core/export/u/3/so/space-a/-/docs/demo/-/assets',
    })

    const selectionTarget = buildUnixFSSelectionDownloadDragTarget({
      sessionIndex: 5,
      sharedObjectId: 'space-b',
      objectKey: 'docs/demo',
      currentPath: '/nested',
      entries: [
        buildEntry('b', 'beta.txt', false),
        buildEntry('a', 'alpha', true),
      ],
    })

    expect(selectionTarget?.mimeType).toBe('application/zip')
    expect(selectionTarget?.filename).toBe('selection.zip')
    expect(selectionTarget?.url).toMatch(
      /^\/p\/spacewave-core\/export-batch\/u\/5\/so\/space-b\/-\/docs\/demo\/-\/nested\/.+\/selection\.zip$/,
    )
  })
})
