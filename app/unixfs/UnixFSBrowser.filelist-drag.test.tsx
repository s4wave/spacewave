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

vi.mock('@s4wave/web/hooks/useUnixFSHandle.js', () => ({
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
  useUnixFSHandleEntries: () => buildResource(h.mockFileEntries),
  useUnixFSHandleStat: () => buildResource(h.mockStat),
}))

vi.mock('@s4wave/web/editors/file-browser/Toolbar.js', () => ({
  Toolbar: () => <div>Toolbar</div>,
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

describe('UnixFSBrowser real file-list drag-to-folder move', () => {
  beforeEach(() => {
    h.mockAddFiles.mockReset()
    h.mockExtractNativeUploadSelection.mockReset()
    h.mockAppStartupBoundary.mockReset()
    h.mockLookup.mockReset()
    h.mockMkdirAll.mockReset()
    h.mockMknod.mockReset()
    h.mockNavigate.mockReset()
    h.mockPathRename.mockReset()
    h.mockRootCloneRename.mockReset()
    h.mockRootClone.mockReset()
    h.mockRootLookupPath.mockReset()

    h.mockRootClone.mockResolvedValue(
      buildDisposableHandle({
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
  })

  it('drops a UnixFS file row onto a folder row through FileList and invokes the move owner', async () => {
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    await waitFor(() => {
      expect(screen.getByText('docs')).toBeTruthy()
      expect(screen.getByText('file.txt')).toBeTruthy()
    })

    const fileRow = screen.getByText('file.txt').closest('[role="row"]')
    const folderRow = screen.getByText('docs').closest('[role="row"]')
    expect(fileRow).toBeTruthy()
    expect(folderRow).toBeTruthy()

    const dataTransfer = createMutableDataTransfer()
    fireEvent.dragStart(fileRow!, { dataTransfer })

    expect(fireEvent.dragOver(folderRow!, { dataTransfer })).toBe(false)
    const latestFolderRow = screen.getByText('docs').closest('[role="row"]')
    expect(latestFolderRow).toBeTruthy()
    fireEvent.drop(latestFolderRow!, { dataTransfer })

    await waitFor(() => {
      expect(h.mockRootLookupPath).toHaveBeenCalledWith('docs', undefined)
      expect(h.mockPathRename.mock.calls).toEqual([
        ['file.txt', 'file.txt', 77, undefined],
      ])
      expect(h.mockRootClone).not.toHaveBeenCalled()
      expect(h.mockRootCloneRename).not.toHaveBeenCalled()
    })
  })
})
