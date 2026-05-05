import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { DriveTypeID } from '@s4wave/sdk/space/drive/drive.js'
import type { Drive } from '@s4wave/sdk/space/drive/drive.pb.js'

const mocks = vi.hoisted(() => ({
  useAccessTypedHandle: vi.fn(),
  useStreamingResource: vi.fn(),
  usePath: vi.fn(),
}))

let currentDrive: Drive | undefined

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: mocks.useAccessTypedHandle,
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: mocks.useStreamingResource,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  usePath: mocks.usePath,
}))

vi.mock('../unixfs/UnixFSBrowser.js', () => ({
  UnixFSBrowser: (props: {
    unixfsId: string
    basePath: string
    currentPath: string
  }) => (
    <div
      data-testid="unixfs-browser"
      data-unixfs-id={props.unixfsId}
      data-base-path={props.basePath}
      data-current-path={props.currentPath}
    />
  ),
}))

import { DriveViewer } from './DriveViewer.js'

const objectInfo = {
  info: {
    case: 'worldObjectInfo',
    value: {
      objectKey: 'drive',
      objectType: DriveTypeID,
    },
  },
}

describe('DriveViewer', () => {
  beforeEach(() => {
    currentDrive = {
      roots: [
        {
          rootId: 'default',
          name: 'My Files',
          rootObjectKey: 'files',
          rootType: 'unixfs/fs-node',
        },
      ],
    }
    mocks.usePath.mockReturnValue('/docs/readme.md')
    mocks.useAccessTypedHandle.mockReturnValue({
      value: { id: 'drive-handle' },
    })
    mocks.useStreamingResource.mockImplementation(() => ({
      value: currentDrive,
      loading: !currentDrive,
    }))
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('accesses the Drive typed handle and delegates the first root to UnixFSBrowser', () => {
    render(
      <DriveViewer
        objectInfo={objectInfo as never}
        worldState={{ value: {} } as never}
      />,
    )

    expect(mocks.useAccessTypedHandle).toHaveBeenCalledWith(
      { value: {} },
      'drive',
      expect.any(Function),
      DriveTypeID,
    )
    const browser = screen.getByTestId('unixfs-browser')
    expect(browser.getAttribute('data-unixfs-id')).toBe('files')
    expect(browser.getAttribute('data-base-path')).toBe('/')
    expect(browser.getAttribute('data-current-path')).toBe('/docs/readme.md')
  })

  it('renders a repair state when the Drive has no backing roots', () => {
    currentDrive = { roots: [] }

    render(
      <DriveViewer
        objectInfo={objectInfo as never}
        worldState={{ value: {} } as never}
      />,
    )

    expect(screen.getByText('Drive has no storage root')).toBeDefined()
  })
})
