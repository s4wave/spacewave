import { describe, expect, it } from 'vitest'

import {
  DOWNLOAD_URL_DRAG_FORMAT,
  buildDownloadURLDragData,
  isDownloadURLDragSupported,
  sanitizeDownloadDragFilename,
  writeDownloadURLDragTarget,
} from './download-url-drag.js'

describe('download url drag', () => {
  it('enables Chrome and Safari class browsers but not Firefox', () => {
    expect(
      isDownloadURLDragSupported(
        'Mozilla/5.0 AppleWebKit/537.36 Chrome/124.0.0.0 Safari/537.36',
      ),
    ).toBe(true)
    expect(
      isDownloadURLDragSupported(
        'Mozilla/5.0 AppleWebKit/605.1.15 Version/17.5 Safari/605.1.15',
      ),
    ).toBe(true)
    expect(
      isDownloadURLDragSupported('Mozilla/5.0 Gecko/20100101 Firefox/124.0'),
    ).toBe(false)
  })

  it('sanitizes delimiter and path characters from filenames', () => {
    expect(sanitizeDownloadDragFilename('a:b/c\\d\r\ne.txt')).toBe(
      'a-b-c-d-e.txt',
    )
    expect(sanitizeDownloadDragFilename('   ')).toBe('download')
  })

  it('builds a DownloadURL payload with an absolute URL', () => {
    expect(
      buildDownloadURLDragData(
        {
          mimeType: 'application/zip',
          filename: 'docs:bundle.zip',
          url: '/p/spacewave-core/export/u/1/so/space/-/docs',
        },
        'https://example.test/u/1/so/space',
      ),
    ).toBe(
      'application/zip:docs-bundle.zip:https://example.test/p/spacewave-core/export/u/1/so/space/-/docs',
    )
  })

  it('writes the DownloadURL payload to DataTransfer', () => {
    const writes = new Map<string, string>()
    writeDownloadURLDragTarget(
      {
        setData: (format, data) => writes.set(format, data),
      },
      {
        mimeType: 'text/plain',
        filename: 'hello.txt',
        url: '/download/hello.txt',
      },
      'https://example.test/app',
    )

    expect(writes.get(DOWNLOAD_URL_DRAG_FORMAT)).toBe(
      'text/plain:hello.txt:https://example.test/download/hello.txt',
    )
  })
})
