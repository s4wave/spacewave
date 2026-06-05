import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentType } from 'react'

import { DriveViewer } from './DriveViewer.js'

const h = vi.hoisted(() => ({
  currentPath: '/',
  entries: [{ id: 'guide', name: 'getting-started.md', isDir: false }],
  mockInvokeCommand: vi.fn(),
  mockNewFolder: vi.fn(),
  mockOpen: vi.fn(),
  mockUploadFiles: vi.fn(),
  canManageSpace: true,
  latestUnixFSBrowserProps: null as null | {
    unixfsId: string
    basePath: string
    currentPath: string
    directoryHeader?: ComponentType<{
      currentPath: string
      entries: Array<{ id: string; name: string; isDir: boolean }>
      onNewFolder: () => void
      onOpen: (
        entries: Array<{ id: string; name: string; isDir: boolean }>,
      ) => void
      onUploadFiles: () => void
    }>
  },
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  usePath: () => h.currentPath,
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContextSafe: () => ({
      spaceSharingState: { canManage: h.canManageSpace },
    }),
  },
}))

vi.mock('@s4wave/web/command/CommandContext.js', () => ({
  useInvokeCommand: () => h.mockInvokeCommand,
}))

vi.mock('./UnixFSBrowser.js', () => ({
  UnixFSBrowser: (props: {
    unixfsId: string
    basePath: string
    currentPath: string
    directoryHeader?: ComponentType<{
      currentPath: string
      entries: Array<{ id: string; name: string; isDir: boolean }>
      onNewFolder: () => void
      onOpen: (
        entries: Array<{ id: string; name: string; isDir: boolean }>,
      ) => void
      onUploadFiles: () => void
    }>
  }) => {
    h.latestUnixFSBrowserProps = props
    const Header = props.directoryHeader
    return (
      <div data-testid="unixfs-browser">
        {Header ? (
          <Header
            currentPath={props.currentPath}
            entries={h.entries}
            onNewFolder={h.mockNewFolder}
            onOpen={h.mockOpen}
            onUploadFiles={h.mockUploadFiles}
          />
        ) : null}
      </div>
    )
  },
}))

describe('DriveViewer', () => {
  beforeEach(() => {
    h.currentPath = '/'
    h.entries = [{ id: 'guide', name: 'getting-started.md', isDir: false }]
    h.canManageSpace = true
    h.mockInvokeCommand.mockReset()
    h.mockNewFolder.mockReset()
    h.mockOpen.mockReset()
    h.mockUploadFiles.mockReset()
    h.latestUnixFSBrowserProps = null
  })

  afterEach(() => {
    cleanup()
  })

  it('wraps the Drive root with getting-started guidance', () => {
    render(
      <DriveViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: { objectKey: 'files', objectType: 'unixfs/fs-node' },
          },
        }}
        worldState={{
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    expect(h.latestUnixFSBrowserProps?.unixfsId).toBe('files')
    expect(screen.getByTestId('drive-welcome')).toBeTruthy()
    expect(screen.getByText('Welcome to your Drive')).toBeTruthy()
  })

  it('does not add Drive guidance to non-Drive UnixFS objects', () => {
    render(
      <DriveViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: { objectKey: 'assets', objectType: 'unixfs/fs-node' },
          },
        }}
        worldState={{
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    expect(h.latestUnixFSBrowserProps?.unixfsId).toBe('assets')
    expect(screen.queryByTestId('drive-welcome')).toBeNull()
  })

  it('hides guidance after user content appears or the starter guide is gone', () => {
    h.entries = [
      { id: 'guide', name: 'getting-started.md', isDir: false },
      { id: 'docs', name: 'docs', isDir: true },
    ]

    const { rerender } = render(
      <DriveViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: { objectKey: 'files', objectType: 'unixfs/fs-node' },
          },
        }}
        worldState={{
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    expect(screen.queryByTestId('drive-welcome')).toBeNull()

    h.entries = []
    rerender(
      <DriveViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: { objectKey: 'files', objectType: 'unixfs/fs-node' },
          },
        }}
        worldState={{
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    expect(screen.queryByTestId('drive-welcome')).toBeNull()
  })

  it('dismisses when starter actions are used', async () => {
    const user = userEvent.setup()
    render(
      <DriveViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: { objectKey: 'files', objectType: 'unixfs/fs-node' },
          },
        }}
        worldState={{
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    await user.click(screen.getByTestId('drive-open-guide-cta'))

    expect(h.mockOpen).toHaveBeenCalledWith([
      { id: 'guide', name: 'getting-started.md', isDir: false },
    ])
    expect(screen.queryByTestId('drive-welcome')).toBeNull()
  })

  it('supports dismiss and hides invite when sharing cannot be managed', async () => {
    const user = userEvent.setup()
    h.canManageSpace = false

    render(
      <DriveViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: { objectKey: 'files', objectType: 'unixfs/fs-node' },
          },
        }}
        worldState={{
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    expect(screen.queryByTestId('drive-invite-cta')).toBeNull()

    await user.click(screen.getByLabelText('Dismiss getting started'))

    expect(screen.queryByTestId('drive-welcome')).toBeNull()
  })
})
