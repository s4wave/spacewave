import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { UnixFSBrowser } from './UnixFSBrowser.js'

const h = vi.hoisted(() => ({
  mockAddFiles: vi.fn(),
  mockExtractNativeUploadSelection: vi.fn(),
  mockAppStartupBoundary: vi.fn(),
  mockLookup: vi.fn(),
  mockMkdirAll: vi.fn(),
  mockMknod: vi.fn(),
  mockNavigate: vi.fn(),
  mockPathRename: vi.fn(),
  mockRootCloneRename: vi.fn(),
  mockRootClone: vi.fn(),
  mockRootLookupPath: vi.fn(),
  mockSyncStatus: {
    packRangeLabel: '2 / 128 KiB',
    packIndexTailLabel: '1 / 4 KiB',
    packLookupLabel: '1 opened / 2 candidates',
    packIndexCacheLabel: '0 hits / 1 misses',
  },
  registeredCommands: new Map<string, unknown>(),
  mockFileEntries: [
    { id: 'docs', name: 'docs', isDir: true },
    { id: 'file', name: 'file.txt', isDir: false },
  ],
  mockStat: {
    info: { isDir: true },
    mimeType: 'inode/directory',
  },
}))

function buildResource<T>(value: T) {
  return {
    value,
    loading: false,
    error: null,
    retry: vi.fn(),
  }
}

function buildDisposableHandle<T extends object>(
  value: T,
): T & {
  [Symbol.dispose]: () => void
} {
  return {
    [Symbol.dispose]: () => undefined,
    ...value,
  }
}

function createMutableDataTransfer(): DataTransfer {
  const data = new Map<string, string>()
  const types: string[] = []
  return {
    dropEffect: 'none',
    effectAllowed: '',
    files: [] as unknown as FileList,
    items: [{ kind: 'string' }] as unknown as DataTransferItemList,
    types,
    setData: vi.fn((format: string, value: string) => {
      data.set(format, value)
      if (!types.includes(format)) types.push(format)
    }),
    getData: vi.fn((format: string) => data.get(format) ?? ''),
    clearData: vi.fn((format?: string) => {
      if (format) {
        data.delete(format)
        const index = types.indexOf(format)
        if (index >= 0) types.splice(index, 1)
        return
      }
      data.clear()
      types.length = 0
    }),
    setDragImage: vi.fn(),
  } as unknown as DataTransfer
}

vi.mock('@s4wave/web/hooks/useUnixFSHandle.js', async (importOriginal) => ({
  getMimeType: (
    await importOriginal<
      typeof import('@s4wave/web/hooks/useUnixFSHandle.js')
    >()
  ).getMimeType,
  useUnixFSRootHandle: () =>
    buildResource({
      clone: h.mockRootClone,
      lookupPath: h.mockRootLookupPath,
    }),
  useUnixFSHandle: () =>
    buildResource({
      lookup: h.mockLookup,
      mkdirAll: h.mockMkdirAll,
      mknod: h.mockMknod,
      rename: h.mockPathRename,
    }),
  useUnixFSHandleListing: () =>
    buildResource({ entries: h.mockFileEntries, rows: h.mockFileEntries }),
  useUnixFSHandleStat: () => buildResource(h.mockStat),
}))

vi.mock('./UnixFSContextMenu.js', () => ({
  UnixFSContextMenu: () => null,
}))

vi.mock('./UnixFSMoveDialog.js', () => ({
  UnixFSMoveDialog: () => null,
}))

vi.mock('./UnixFSFileViewer.js', () => ({
  UnixFSFileViewer: () => <div>Viewer</div>,
}))

vi.mock('@s4wave/app/session/SessionUploadManagerContext.js', () => ({
  useSessionUploadManager: () => ({
    addFiles: h.mockAddFiles,
  }),
}))

vi.mock('./native-upload.js', () => ({
  extractNativeUploadSelection: h.mockExtractNativeUploadSelection,
}))

vi.mock('@s4wave/web/command/useCommand.js', () => ({
  useCommand: (config: { commandId: string }) => {
    h.registeredCommands.set(config.commandId, config)
  },
}))

vi.mock('@s4wave/web/contexts/TabActiveContext.js', () => ({
  useIsTabActive: () => true,
}))

vi.mock('@s4wave/web/object/TabContext.js', () => ({
  useTabContext: () => null,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => h.mockNavigate,
}))

vi.mock('@s4wave/web/router/HistoryRouter.js', () => ({
  useHistory: () => null,
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContextSafe: () => ({
      spaceId: 'space-test',
      spaceSharingState: { canManage: true },
    }),
  },
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  useSessionIndex: () => 1,
}))

vi.mock('../session/SessionSyncStatusContext.js', () => ({
  useSessionSyncStatus: () => h.mockSyncStatus,
}))

vi.mock('@s4wave/app/quickstart/startup-boundary.js', () => ({
  markAppStartupBoundary: h.mockAppStartupBoundary,
}))

class ResizeObserverMock {
  observe() {}
  unobserve() {}
  disconnect() {}
}

// renderAt renders the browser at currentPath.
function renderAt(currentPath: string) {
  render(
    <UnixFSBrowser
      unixfsId="files"
      basePath="/"
      currentPath={currentPath}
      worldState={buildResource({} as IWorldState)}
    />,
  )
}

// dragFileTo drags the file.txt row onto the target found by findTarget,
// which runs again before the drop to pick up a rerendered target.
function dragFileTo(findTarget: () => Element | null) {
  // Start the drag on the file row.
  const fileRow = screen.getByText('file.txt').closest('[role="row"]')
  expect(fileRow).toBeTruthy()
  const dataTransfer = createMutableDataTransfer()
  fireEvent.dragStart(fileRow!, { dataTransfer })

  // The target accepts the drag, then takes the drop.
  expect(fireEvent.dragOver(findTarget()!, { dataTransfer })).toBe(false)
  const target = findTarget()
  expect(target).toBeTruthy()
  fireEvent.drop(target!, { dataTransfer })
}

describe('UnixFSBrowser real file-list drag-to-folder move', () => {
  beforeEach(() => {
    // Reset every mock.
    for (const mock of [
      h.mockAddFiles,
      h.mockExtractNativeUploadSelection,
      h.mockAppStartupBoundary,
      h.mockLookup,
      h.mockMkdirAll,
      h.mockMknod,
      h.mockNavigate,
      h.mockPathRename,
      h.mockRootCloneRename,
      h.mockRootClone,
      h.mockRootLookupPath,
    ]) {
      mock.mockReset()
    }

    // Give the virtualized file list a layout.
    vi.stubGlobal('ResizeObserver', ResizeObserverMock)
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
      configurable: true,
      get: () => 640,
    })

    // Resolve the root clone and the docs lookup to distinct handles.
    h.mockRootClone.mockResolvedValue(
      buildDisposableHandle({
        id: 11,
        rename: h.mockRootCloneRename,
      }),
    )
    h.mockRootLookupPath.mockResolvedValue({
      handle: buildDisposableHandle({
        id: 77,
      }),
    })
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('drops a UnixFS file row onto a folder row through FileList and invokes the move owner', async () => {
    // Show the root listing.
    renderAt('/')
    await waitFor(() => {
      expect(screen.getByText('docs')).toBeTruthy()
      expect(screen.getByText('file.txt')).toBeTruthy()
    })

    // Drop the file on the docs row.
    dragFileTo(() => screen.getByText('docs').closest('[role="row"]'))

    // The file moves into the looked-up docs handle.
    await waitFor(() => {
      expect(h.mockRootLookupPath).toHaveBeenCalledWith('docs', undefined)
      expect(h.mockPathRename.mock.calls).toEqual([
        ['file.txt', 'file.txt', 77, undefined],
      ])
      expect(h.mockRootClone).not.toHaveBeenCalled()
      expect(h.mockRootCloneRename).not.toHaveBeenCalled()
    })
  })

  it('drops a UnixFS file row onto the root path target through the move owner', async () => {
    // Show the docs listing.
    renderAt('/docs')
    await waitFor(() => {
      expect(screen.getByText('file.txt')).toBeTruthy()
      expect(screen.getByLabelText('Navigate to root')).toBeTruthy()
    })

    // Drop the file on the root breadcrumb.
    dragFileTo(() => screen.getByLabelText('Navigate to root'))

    // The file moves into a clone of the root handle.
    await waitFor(() => {
      expect(h.mockRootClone).toHaveBeenCalledWith(undefined)
      expect(h.mockPathRename.mock.calls).toEqual([
        ['file.txt', 'file.txt', 11, undefined],
      ])
      expect(h.mockRootLookupPath).not.toHaveBeenCalled()
    })
  })

  it('drops a UnixFS file row onto the parent breadcrumb through the move owner', async () => {
    // Show the docs/images listing.
    const parent = () =>
      screen.getByRole('button', { name: 'Navigate to docs' })
    renderAt('/docs/images')
    await waitFor(() => {
      expect(screen.getByText('file.txt')).toBeTruthy()
      expect(parent()).toBeTruthy()
    })

    // Drop the file on the docs breadcrumb.
    dragFileTo(parent)

    // The file moves into the looked-up docs handle.
    await waitFor(() => {
      expect(h.mockRootLookupPath).toHaveBeenCalledWith('docs', undefined)
      expect(h.mockPathRename.mock.calls).toEqual([
        ['file.txt', 'file.txt', 77, undefined],
      ])
      expect(h.mockRootClone).not.toHaveBeenCalled()
    })
  })

  it('drops a UnixFS file row onto the Up control through the move owner', async () => {
    // Show the docs/images listing.
    const up = () => screen.getByRole('button', { name: 'Up' })
    renderAt('/docs/images')
    await waitFor(() => {
      expect(screen.getByText('file.txt')).toBeTruthy()
      expect(up()).toBeTruthy()
    })

    // Drop the file on the Up control.
    dragFileTo(up)

    // The file moves into the looked-up parent handle.
    await waitFor(() => {
      expect(h.mockRootLookupPath).toHaveBeenCalledWith('docs', undefined)
      expect(h.mockPathRename.mock.calls).toEqual([
        ['file.txt', 'file.txt', 77, undefined],
      ])
      expect(h.mockRootClone).not.toHaveBeenCalled()
    })
  })
})
