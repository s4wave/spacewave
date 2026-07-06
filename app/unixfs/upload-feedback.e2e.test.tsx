import { useEffect, useRef } from 'react'
import { describe, expect, it } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'

import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'
import { ViewerFrame } from '@s4wave/web/frame/ViewerFrame.js'
import {
  SessionUploadManagerProvider,
  SessionUploadIndicator,
  useSessionUploadManager,
} from '@s4wave/app/session/SessionUploadManagerContext.js'

// recordingHandle stands in for a UnixFS handle. It records the abort signal of
// each upload so a test can prove the write was, or was not, aborted. When
// resolveUpload is false the tree upload never settles, holding the upload
// "uploading"; when true it resolves, firing the completion popover.
function recordingHandle(resolveUpload: boolean): {
  handle: FSHandle
  signals: AbortSignal[]
} {
  const signals: AbortSignal[] = []
  const uploadTree = (
    _entries: unknown,
    _opts: unknown,
    signal?: AbortSignal,
  ) => {
    if (signal) signals.push(signal)
    return resolveUpload
      ? Promise.resolve({
          bytesWritten: 0n,
          filesWritten: 2n,
          directoriesWritten: 0n,
        })
      : new Promise<never>(() => {})
  }
  return { handle: { uploadTree } as unknown as FSHandle, signals }
}

// DriveViewerStub starts a two-file upload against handle on mount, standing in
// for the folder view the returning user drops files onto. Unmounting it is the
// "navigate away from the folder" event the session-owned manager must survive.
function DriveViewerStub({ handle }: { handle: FSHandle }) {
  const manager = useSessionUploadManager()
  const startedRef = useRef(false)
  useEffect(() => {
    if (startedRef.current || !manager) return
    startedRef.current = true
    manager.addFiles(handle, [
      new File(['a mountain photo'], 'sunset.jpg'),
      new File(['trip planning notes'], 'itinerary.txt'),
    ])
  }, [manager, handle])
  return (
    <div className="grid flex-1 grid-cols-4 content-start gap-4 p-6">
      {['Documents', 'Photos', 'Music', 'Projects', 'Archive', 'Shared'].map(
        (name) => (
          <div
            key={name}
            className="border-frame-overlay-border bg-frame-overlay flex h-24 flex-col justify-end rounded-md border p-3"
          >
            <span className="text-foreground text-sm">{name}</span>
            <span className="text-foreground-alt text-xs">Folder</span>
          </div>
        ),
      )}
    </div>
  )
}

// CancelAllControl gives the test an explicit user action to cancel uploads,
// the one place cancelAll is allowed to fire besides session teardown.
function CancelAllControl() {
  const manager = useSessionUploadManager()
  return (
    <button data-testid="cancel-all" onClick={() => manager?.cancelAll()}>
      Cancel all
    </button>
  )
}

// UploadFeedbackSurface renders the session-owned upload indicator over a
// realistic file-browser backdrop. showViewer toggles the folder view so a test
// can navigate away while an upload is in flight.
function UploadFeedbackSurface({
  handle,
  showViewer,
}: {
  handle: FSHandle
  showViewer: boolean
}) {
  return (
    <SessionUploadManagerProvider>
      <div className="bg-background text-foreground fixed inset-0 flex flex-col">
        <BottomBarRoot openMenu="" setOpenMenu={() => {}}>
          <SessionUploadIndicator />
          <CancelAllControl />
          <ViewerFrame>
            {showViewer ? (
              <DriveViewerStub handle={handle} />
            ) : (
              <div className="text-foreground p-6">A different object</div>
            )}
          </ViewerFrame>
        </BottomBarRoot>
      </div>
    </SessionUploadManagerProvider>
  )
}

async function capture(name: string) {
  return page.screenshot({
    path: `__screenshots__/upload-feedback/${name}.png`,
  })
}

describe('upload feedback popovers', () => {
  it('anchors the start notification to the bottom-right indicator', async () => {
    const { handle } = recordingHandle(false)
    await render(<UploadFeedbackSurface handle={handle} showViewer={true} />)

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
    const { handle } = recordingHandle(true)
    await render(<UploadFeedbackSurface handle={handle} showViewer={true} />)

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

  it('keeps the upload and its indicator alive when the folder view unmounts', async () => {
    const { handle, signals } = recordingHandle(false)
    const view = await render(
      <UploadFeedbackSurface handle={handle} showViewer={true} />,
    )

    // Upload is in flight: the indicator reports the active count.
    await expect.element(page.getByText('Uploading 2/2')).toBeInTheDocument()
    expect(signals).toHaveLength(1)

    // Navigate away from the folder: the viewer unmounts, the session owner does
    // not. The indicator must persist and the write must not be aborted.
    await view.rerender(
      <UploadFeedbackSurface handle={handle} showViewer={false} />,
    )
    await expect
      .element(page.getByText('A different object'))
      .toBeInTheDocument()
    await expect.element(page.getByText('Uploading 2/2')).toBeInTheDocument()
    expect(signals[0].aborted).toBe(false)

    await capture('survives-navigation')
    await cleanup()
  })

  it('aborts uploads on explicit cancel-all', async () => {
    const { handle, signals } = recordingHandle(false)
    await render(<UploadFeedbackSurface handle={handle} showViewer={true} />)

    await expect.element(page.getByText('Uploading 2/2')).toBeInTheDocument()

    await page.getByTestId('cancel-all').click()

    await expect
      .element(page.getByText('Uploading 2/2'))
      .not.toBeInTheDocument()
    expect(signals[0].aborted).toBe(true)

    await cleanup()
  })
})
