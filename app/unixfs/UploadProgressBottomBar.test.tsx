import React from 'react'
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'
import { ViewerFrame } from '@s4wave/web/frame/ViewerFrame.js'
import type { UploadManager } from './useUploadManager.js'
import { UploadProgressBottomBar } from './UploadProgressBottomBar.js'

function buildUploadManager(
  overrides: Partial<UploadManager> = {},
): UploadManager {
  return {
    items: [],
    activeCount: 0,
    addFiles: vi.fn(),
    cancelUpload: vi.fn(),
    cancelAll: vi.fn(),
    clearDone: vi.fn(),
    ...overrides,
  }
}

describe('UploadProgressBottomBar', () => {
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

    expect(screen.getByText('2/2 uploaded')).toBeTruthy()
  })
})
