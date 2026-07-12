import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'
import { ViewerFrame } from '@s4wave/web/frame/ViewerFrame.js'
import type { UploadItem, UploadManager } from './useUploadManager.js'
import { UploadProgressBottomBar } from './UploadProgressBottomBar.js'

function buildUploadManager(
  overrides: Partial<UploadManager> = {},
): UploadManager {
  return {
    items: [],
    activeCount: 0,
    lastEvent: null,
    addFiles: vi.fn(),
    cancelUpload: vi.fn(),
    cancelAll: vi.fn(),
    clearDone: vi.fn(),
    ...overrides,
  }
}

function uploadingItem(): UploadItem {
  return {
    id: 'upload-1',
    groupId: 'group-1',
    kind: 'file',
    file: null,
    name: 'alpha.txt',
    path: 'docs/alpha.txt',
    totalSize: 100,
    bytesWritten: 40,
    status: 'uploading',
    abortController: new AbortController(),
  }
}

function doneItem(id: string, name: string): UploadItem {
  return {
    id,
    groupId: id,
    kind: 'file',
    file: null,
    name,
    path: `docs/${name}`,
    totalSize: 100,
    bytesWritten: 100,
    status: 'done',
    abortController: new AbortController(),
  }
}

describe('UploadProgressBottomBar', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders a full-view overlay with aggregate upload progress', () => {
    const uploadManager = buildUploadManager({
      activeCount: 1,
      items: [
        {
          id: 'upload-1',
          groupId: 'group-1',
          kind: 'file',
          file: null,
          name: 'alpha.txt',
          path: 'docs/alpha.txt',
          totalSize: 100,
          bytesWritten: 40,
          status: 'uploading',
          abortController: new AbortController(),
        },
        {
          id: 'upload-2',
          groupId: 'group-2',
          kind: 'directory',
          file: null,
          name: 'assets',
          path: 'media/assets',
          totalSize: 0,
          bytesWritten: 0,
          status: 'done',
          abortController: new AbortController(),
        },
        {
          id: 'upload-3',
          groupId: 'group-3',
          kind: 'file',
          file: null,
          name: 'broken.png',
          path: 'images/broken.png',
          totalSize: 50,
          bytesWritten: 10,
          status: 'error',
          error: 'Network failed',
          abortController: new AbortController(),
        },
      ],
    })

    render(
      <BottomBarRoot openMenu="upload-progress" setOpenMenu={() => {}}>
        <UploadProgressBottomBar uploadManager={uploadManager} />
        <ViewerFrame>
          <div>Browser</div>
        </ViewerFrame>
      </BottomBarRoot>,
    )

    const overlay = screen.getByTestId('upload-progress-overlay')
    expect(overlay.className).toContain('h-full')
    expect(overlay.className).toContain('w-full')
    expect(screen.getByText('Uploads')).toBeTruthy()
    expect(screen.getByText('33%')).toBeTruthy()
    expect(screen.getByText('1 uploading')).toBeTruthy()
    expect(screen.getByText('50 B of 150 B')).toBeTruthy()
    expect(screen.getByText('3 items')).toBeTruthy()
    expect(screen.getByText('docs/alpha.txt')).toBeTruthy()
    expect(screen.getByText('media/assets')).toBeTruthy()
    expect(screen.getByText('Network failed')).toBeTruthy()
  })

  it('keeps the compact bottom-bar summary for the button state', () => {
    const uploadManager = buildUploadManager({
      items: [
        {
          id: 'upload-1',
          groupId: 'group-1',
          kind: 'file',
          file: null,
          name: 'alpha.txt',
          path: 'docs/alpha.txt',
          totalSize: 100,
          bytesWritten: 100,
          status: 'done',
          abortController: new AbortController(),
        },
        {
          id: 'upload-2',
          groupId: 'group-2',
          kind: 'file',
          file: null,
          name: 'beta.txt',
          path: 'docs/beta.txt',
          totalSize: 50,
          bytesWritten: 50,
          status: 'done',
          abortController: new AbortController(),
        },
      ],
    })

    render(
      <BottomBarRoot openMenu="" setOpenMenu={() => {}}>
        <UploadProgressBottomBar uploadManager={uploadManager} />
        <ViewerFrame>
          <div>Browser</div>
        </ViewerFrame>
      </BottomBarRoot>,
    )

    const label = screen.getByText('2/2 uploaded')
    const button = label.closest('button')
    expect(label).toBeTruthy()
    expect(button?.className).toContain('whitespace-nowrap')
    expect(button?.className).toContain('shrink-0')
  })

  it('keeps the active upload summary on one line', () => {
    const uploadManager = buildUploadManager({
      activeCount: 1,
      items: [
        {
          id: 'upload-1',
          groupId: 'group-1',
          kind: 'file',
          file: null,
          name: 'alpha.txt',
          path: 'docs/alpha.txt',
          totalSize: 100,
          bytesWritten: 50,
          status: 'uploading',
          abortController: new AbortController(),
        },
      ],
    })

    render(
      <BottomBarRoot openMenu="" setOpenMenu={() => {}}>
        <UploadProgressBottomBar uploadManager={uploadManager} />
        <ViewerFrame>
          <div>Browser</div>
        </ViewerFrame>
      </BottomBarRoot>,
    )

    const label = screen.getByText('Uploading 1/1')
    const button = label.closest('button')
    expect(label).toBeTruthy()
    expect(button?.className).toContain('whitespace-nowrap')
    expect(button?.className).toContain('shrink-0')
  })

  it('shows an anchored start notification on an upload started event', async () => {
    const uploadManager = buildUploadManager({
      activeCount: 1,
      items: [uploadingItem()],
      lastEvent: { id: 1, kind: 'started', fileCount: 2, errorCount: 0 },
    })

    render(
      <BottomBarRoot openMenu="" setOpenMenu={() => {}}>
        <UploadProgressBottomBar uploadManager={uploadManager} />
        <ViewerFrame>
          <div>Browser</div>
        </ViewerFrame>
      </BottomBarRoot>,
    )

    expect(await screen.findByTestId('upload-feedback-popover')).toBeTruthy()
    expect(screen.getByText('Upload started')).toBeTruthy()
    expect(
      screen.getByText('Uploading 2 files. Track progress here.'),
    ).toBeTruthy()
  })

  it('shows an anchored completion notification on an all-finished event', async () => {
    const uploadManager = buildUploadManager({
      items: [
        doneItem('upload-1', 'alpha.txt'),
        doneItem('upload-2', 'beta.txt'),
      ],
      lastEvent: { id: 1, kind: 'completed', fileCount: 2, errorCount: 0 },
    })

    render(
      <BottomBarRoot openMenu="" setOpenMenu={() => {}}>
        <UploadProgressBottomBar uploadManager={uploadManager} />
        <ViewerFrame>
          <div>Browser</div>
        </ViewerFrame>
      </BottomBarRoot>,
    )

    expect(await screen.findByTestId('upload-feedback-popover')).toBeTruthy()
    expect(screen.getByText('Upload complete')).toBeTruthy()
    expect(screen.getByText('2 files added to this folder.')).toBeTruthy()
  })

  it('replaces a quota error chain with actionable storage guidance', () => {
    const uploadManager = buildUploadManager({
      items: [
        {
          id: 'upload-quota',
          groupId: 'group-quota',
          kind: 'file',
          file: null,
          name: 'photo.jpg',
          path: 'photos/photo.jpg',
          totalSize: 100,
          bytesWritten: 50,
          status: 'error',
          error:
            'Server error: build segment: browser storage quota exceeded (usage 100 bytes, quota 150 bytes, available 50 bytes, write 100 bytes): QuotaExceededError',
          abortController: new AbortController(),
        },
      ],
    })

    render(
      <BottomBarRoot openMenu="upload-progress" setOpenMenu={() => {}}>
        <UploadProgressBottomBar uploadManager={uploadManager} />
        <ViewerFrame>
          <div>Browser</div>
        </ViewerFrame>
      </BottomBarRoot>,
    )

    expect(
      screen.getByText(
        'Browser storage quota was exceeded (50 B available; 100 B needed). Free storage used by this site or device, then retry. Spacewave already requested persistent storage; persistence prevents eviction but does not increase capacity.',
      ),
    ).toBeTruthy()
    expect(screen.queryByText(/build segment/)).toBeNull()
  })
})
