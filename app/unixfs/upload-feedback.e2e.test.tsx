import { useEffect, useMemo, useRef } from 'react'
import { describe, expect, it } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'

import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'
import { ViewerFrame } from '@s4wave/web/frame/ViewerFrame.js'

import { useUploadManager } from './useUploadManager.js'
import { UploadProgressBottomBar } from './UploadProgressBottomBar.js'

// fakeHandle stands in for a UnixFS handle so the real upload manager drives the
// start/completion event pipeline without a live runtime. When resolveUpload is
// false the tree upload never settles, holding the "started" popover open; when
// true it resolves, firing the "completed" popover.
function fakeHandle(resolveUpload: boolean): FSHandle {
  const uploadTree = resolveUpload
    ? () =>
        Promise.resolve({
          bytesWritten: 0n,
          filesWritten: 2n,
          directoriesWritten: 0n,
        })
    : () => new Promise<never>(() => {})
  return { uploadTree } as unknown as FSHandle
}

// UploadFeedbackSurface renders the real bottom-right upload indicator inside a
// full-viewport ViewerFrame and starts a two-file upload on mount, so the
// anchored feedback popover renders over a realistic file-browser backdrop.
function UploadFeedbackSurface({ resolveUpload }: { resolveUpload: boolean }) {
  const handle = useMemo(() => fakeHandle(resolveUpload), [resolveUpload])
  const uploadManager = useUploadManager(handle, 5)
  const startedRef = useRef(false)
  useEffect(() => {
    if (startedRef.current) return
    startedRef.current = true
    uploadManager.addFiles([
      new File(['a mountain photo'], 'sunset.jpg'),
      new File(['trip planning notes'], 'itinerary.txt'),
    ])
  }, [uploadManager])

  return (
    <div className="bg-background text-foreground fixed inset-0 flex flex-col">
      <BottomBarRoot openMenu="" setOpenMenu={() => {}}>
        <UploadProgressBottomBar uploadManager={uploadManager} />
        <ViewerFrame>
          <div className="grid flex-1 grid-cols-4 content-start gap-4 p-6">
            {[
              'Documents',
              'Photos',
              'Music',
              'Projects',
              'Archive',
              'Shared',
            ].map((name) => (
              <div
                key={name}
                className="border-frame-overlay-border bg-frame-overlay flex h-24 flex-col justify-end rounded-md border p-3"
              >
                <span className="text-foreground text-sm">{name}</span>
                <span className="text-foreground-alt text-xs">Folder</span>
              </div>
            ))}
          </div>
        </ViewerFrame>
      </BottomBarRoot>
    </div>
  )
}

async function capture(name: string) {
  return page.screenshot({
    path: `__screenshots__/upload-feedback/${name}.png`,
  })
}

describe('upload feedback popovers', () => {
  it('anchors the start notification to the bottom-right indicator', async () => {
    await render(<UploadFeedbackSurface resolveUpload={false} />)

    await expect
      .element(page.getByTestId('upload-feedback-popover'))
      .toBeInTheDocument()
    await expect.element(page.getByText('Upload started')).toBeInTheDocument()
    await expect
      .element(page.getByText('Uploading 2 files. Track progress here.'))
      .toBeInTheDocument()

    await capture('start')
    await cleanup()
  })

  it('anchors the completion notification to the bottom-right indicator', async () => {
    await render(<UploadFeedbackSurface resolveUpload={true} />)

    await expect
      .element(page.getByTestId('upload-feedback-popover'))
      .toBeInTheDocument()
    await expect.element(page.getByText('Upload complete')).toBeInTheDocument()
    await expect
      .element(page.getByText('2 files added to this folder.'))
      .toBeInTheDocument()

    await capture('complete')
    await cleanup()
  })
})
