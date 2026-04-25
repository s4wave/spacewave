import type { DragEvent } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { APP_DRAG_MIME, APP_DRAG_VERSION } from '@s4wave/web/dnd/app-drag.js'
import type { DownloadDragTarget } from '@s4wave/web/dnd/download-url-drag.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import type { RenderEntryCallback } from '@s4wave/web/editors/file-browser/FileListEntry.js'

import { UnixFSBrowser } from './UnixFSBrowser.js'
import { buildUnixFSEntryAppDragEnvelope } from './unixfs-app-drag.js'
import type { UnixFSMoveItem } from './move.js'

interface RegisteredCommand {
  commandId: string
  enabled?: boolean
  handler?: () => void
}

interface MockContextMenuProps {
  state?: {
    entry?: FileEntry | null
    actionEntries?: FileEntry[]
    moveItems?: UnixFSMoveItem[]
  }
  onMove?: (moveItems: UnixFSMoveItem[]) => void
  onNewFolder?: () => void
}

interface MockMoveDialogProps {
  moveItems: UnixFSMoveItem[]
  onConfirm?: (destinationPath: string) => Promise<void>
  onOpenChange?: (open: boolean) => void
}

interface MockFileViewerProps {
  inlineFileURL?: string
  path: string
  stat: {
    mimeType: string
  }
}

interface MockFileListProps {
  entries: FileEntry[]
  dropTargetEntryId?: string | null
  renderEntry?: RenderEntryCallback
  getDragEnvelope?: (
    entry: FileEntry,
    context: { selectedIds: string[] },
  ) => {
    version: number
    items: Array<{
      id: string
      label?: string
      capabilities: Array<{ kind: string }>
    }>
  } | null
  getDownloadDragTarget?: (
    entry: FileEntry,
    context: { selectedIds: string[] },
  ) => DownloadDragTarget | null
  onContextMenu?: (item: { data: FileEntry }, event: MouseEvent) => void
  onStateChange?: (state: { selectedIds?: string[] }) => void
  onEntryDragOver?: (
    entry: FileEntry,
    event: DragEvent<HTMLDivElement>,
  ) => boolean
  onEntryDragLeave?: (
    entry: FileEntry,
    event: DragEvent<HTMLDivElement>,
  ) => void
  onEntryDrop?: (entry: FileEntry, event: DragEvent<HTMLDivElement>) => void
}

interface MockToolbarProps {
  onNewFolder?: () => void
}

const h = vi.hoisted(() => ({
  mockAddFiles: vi.fn(),
  mockExtractNativeUploadSelection: vi.fn(),
  mockDownloadUnixFSSelection: vi.fn(),
  mockLookup: vi.fn(),
  mockMkdirAll: vi.fn(),
  mockMknod: vi.fn(),
  mockRootHandleResource: null as ReturnType<typeof buildResource> | null,
  mockPathHandleResource: null as ReturnType<typeof buildResource> | null,
  mockEntriesResource: null as ReturnType<typeof buildResource> | null,
  mockStatResource: null as ReturnType<typeof buildResource> | null,
  mockRename: vi.fn(),
  mockRootClone: vi.fn(),
  mockRootLookupPath: vi.fn(),
  mockSyncStatus: {
    packRangeLabel: '2 / 128 KiB',
    packIndexTailLabel: '1 / 4 KiB',
    packLookupLabel: '1 opened / 2 candidates',
    packIndexCacheLabel: '0 hits / 1 misses',
  },
  registeredCommands: new Map<string, RegisteredCommand>(),
  latestFileListProps: null as MockFileListProps | null,
  latestContextMenuProps: null as MockContextMenuProps | null,
  latestMoveDialogProps: null as MockMoveDialogProps | null,
  latestFileViewerProps: null as MockFileViewerProps | null,
  mockStat: null as {
    info: { isDir?: boolean }
    mimeType: string
  } | null,
  mockFileEntries: [
    { id: 'docs', name: 'docs', isDir: true },
    { id: 'file', name: 'file.txt', isDir: false },
    { id: 'logo', name: 'logo.png', isDir: false },
  ] as FileEntry[],
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

vi.mock('@s4wave/web/hooks/useUnixFSHandle.js', () => ({
  useUnixFSRootHandle: () =>
    h.mockRootHandleResource ??
    buildResource({
      clone: h.mockRootClone,
      lookupPath: h.mockRootLookupPath,
    }),
  useUnixFSHandle: () =>
    h.mockPathHandleResource ??
    buildResource({
      lookup: h.mockLookup,
      mkdirAll: h.mockMkdirAll,
      mknod: h.mockMknod,
    }),
  useUnixFSHandleEntries: () =>
    h.mockEntriesResource ?? buildResource(h.mockFileEntries),
  useUnixFSHandleStat: () => h.mockStatResource ?? buildResource(h.mockStat),
}))

vi.mock('@s4wave/web/editors/file-browser/FileList.js', () => ({
  FileList: (props: MockFileListProps) => {
    h.latestFileListProps = props
    const {
      entries,
      dropTargetEntryId,
      renderEntry,
      onEntryDragOver,
      onEntryDragLeave,
      onEntryDrop,
    } = props
    return (
      <div>
        {entries.map((entry) => {
          const defaultNode = <span>{entry.name}</span>
          return (
            <div
              key={entry.id}
              data-testid={`file-entry-${entry.id}`}
              data-drop-active={
                dropTargetEntryId === entry.id ? 'true' : 'false'
              }
              onDragOver={(event) => {
                if (!onEntryDragOver?.(entry, event)) return
                event.preventDefault()
              }}
              onDragLeave={(event) => onEntryDragLeave?.(entry, event)}
              onDrop={(event) => onEntryDrop?.(entry, event)}
            >
              {renderEntry?.({ entry, defaultNode, path: '/' }) ?? defaultNode}
            </div>
          )
        })}
      </div>
    )
  },
}))

vi.mock('@s4wave/web/editors/file-browser/Toolbar.js', () => ({
  Toolbar: ({ onNewFolder }: MockToolbarProps) => (
    <div>
      Toolbar
      <button onClick={onNewFolder} title="New folder" type="button">
        New folder
      </button>
    </div>
  ),
}))

vi.mock('./UnixFSContextMenu.js', () => ({
  UnixFSContextMenu: (props: MockContextMenuProps) => {
    h.latestContextMenuProps = props
    return null
  },
}))

vi.mock('./UnixFSMoveDialog.js', () => ({
  UnixFSMoveDialog: (props: MockMoveDialogProps) => {
    h.latestMoveDialogProps = props
    return null
  },
}))

vi.mock('./UploadProgressBottomBar.js', () => ({
  UploadProgressBottomBar: () => null,
}))

vi.mock('./UnixFSFileViewer.js', () => ({
  UnixFSFileViewer: (props: MockFileViewerProps) => {
    h.latestFileViewerProps = props
    return <div>Viewer</div>
  },
}))

vi.mock('./useUploadManager.js', () => ({
  useUploadManager: () => ({
    addFiles: h.mockAddFiles,
  }),
}))

vi.mock('./native-upload.js', () => ({
  extractNativeUploadSelection: h.mockExtractNativeUploadSelection,
}))

vi.mock('@s4wave/web/command/useCommand.js', () => ({
  useCommand: (config: RegisteredCommand) => {
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
  useNavigate: () => vi.fn(),
}))

vi.mock('@s4wave/web/router/HistoryRouter.js', () => ({
  useHistory: () => null,
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContextSafe: () => ({ spaceId: 'space-test' }),
  },
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  useSessionIndex: () => 1,
}))

vi.mock('../session/SessionSyncStatusContext.js', () => ({
  useSessionSyncStatus: () => h.mockSyncStatus,
}))

vi.mock('./download.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./download.js')>()
  return {
    ...actual,
    downloadUnixFSSelection: h.mockDownloadUnixFSSelection,
  }
})

function createInternalAppDragDataTransfer() {
  const appDrag = JSON.stringify({
    version: APP_DRAG_VERSION,
    items: [{ id: 'docs', capabilities: [] }],
  })
  return {
    items: [{ kind: 'string' }],
    types: [APP_DRAG_MIME],
    files: [],
    getData: (format: string) => (format === APP_DRAG_MIME ? appDrag : ''),
  }
}

function createUnixFSEntryAppDragDataTransfer({
  unixfsId = 'files',
  currentPath = '/',
  entryName = 'file.txt',
  isDir = false,
}: {
  unixfsId?: string
  currentPath?: string
  entryName?: string
  isDir?: boolean
}) {
  const envelope = buildUnixFSEntryAppDragEnvelope({
    entry: {
      id: 'file',
      name: entryName,
      isDir,
    },
    currentPath,
    sessionIndex: null,
    spaceId: null,
    unixfsId,
  })
  if (!envelope) {
    throw new Error('failed to build UnixFS entry drag envelope')
  }
  return {
    items: [{ kind: 'string' }],
    types: [APP_DRAG_MIME],
    files: [],
    getData: (format: string) =>
      format === APP_DRAG_MIME ? JSON.stringify(envelope) : '',
  }
}

function createInternalTabDragDataTransfer() {
  return {
    items: [{ kind: 'string' }],
    types: ['text/plain'],
    files: [],
    getData: (format: string) =>
      format === 'text/plain' ? '--flexlayout--' : '',
  }
}

function createAppDragDataTransferFromEnvelope(envelope: {
  version: number
  items: Array<{
    id: string
    label?: string
    capabilities: Array<{ kind: string; value?: unknown }>
  }>
}) {
  const json = JSON.stringify(envelope)
  return {
    items: [{ kind: 'string' }],
    types: [APP_DRAG_MIME],
    files: [],
    getData: (format: string) => (format === APP_DRAG_MIME ? json : ''),
  }
}

function createNativeFileDataTransfer() {
  const file = new File(['hello'], 'hello.txt', { type: 'text/plain' })
  return {
    items: [{ kind: 'file' }],
    types: ['Files'],
    files: [file],
  }
}

function setSelection(selectedIds: string[]) {
  const onStateChange = h.latestFileListProps?.onStateChange
  if (!onStateChange) {
    throw new Error('file list did not provide onStateChange')
  }
  onStateChange({ selectedIds })
}

function triggerDownloadCommand() {
  const handler = h.registeredCommands.get('spacewave.file.download')?.handler
  if (!handler) {
    throw new Error('download command was not registered')
  }
  handler()
}

function triggerRowContextMenu(
  entry: FileEntry,
  clientX: number,
  clientY: number,
) {
  const onContextMenu = h.latestFileListProps?.onContextMenu
  if (!onContextMenu) {
    throw new Error('file list did not provide onContextMenu')
  }
  onContextMenu(
    { data: entry },
    new MouseEvent('contextmenu', { clientX, clientY }),
  )
}

function getContextMenuActionEntryIds() {
  return (
    h.latestContextMenuProps?.state?.actionEntries?.map((entry) => entry.id) ??
    []
  )
}

function getContextMenuMoveItemPaths() {
  return (
    h.latestContextMenuProps?.state?.moveItems?.map((item) => item.path) ?? []
  )
}

function getLatestDragEnvelope(entry: FileEntry, selectedIds: string[] = []) {
  return (
    h.latestFileListProps?.getDragEnvelope?.(entry, { selectedIds }) ?? null
  )
}

function getLatestDownloadDragTarget(
  entry: FileEntry,
  selectedIds: string[] = [],
) {
  return (
    h.latestFileListProps?.getDownloadDragTarget?.(entry, { selectedIds }) ??
    null
  )
}

function setUserAgent(userAgent: string) {
  Object.defineProperty(window.navigator, 'userAgent', {
    value: userAgent,
    configurable: true,
  })
}

function triggerContextMenuMove() {
  const moveItems = h.latestContextMenuProps?.state?.moveItems ?? []
  h.latestContextMenuProps?.onMove?.(moveItems)
}

const unixFSLoadingStageCases: Array<[string, () => void, string]> = [
  [
    'root mount',
    () => {
      h.mockRootHandleResource = {
        value: null,
        loading: true,
        error: null,
        retry: vi.fn(),
      }
      h.mockEntriesResource = {
        value: null,
        loading: true,
        error: null,
        retry: vi.fn(),
      }
    },
    'Stage: mounting UnixFS root',
  ],
  [
    'path lookup',
    () => {
      h.mockPathHandleResource = {
        value: null,
        loading: true,
        error: null,
        retry: vi.fn(),
      }
      h.mockEntriesResource = {
        value: null,
        loading: true,
        error: null,
        retry: vi.fn(),
      }
    },
    'Stage: resolving path',
  ],
  [
    'stat',
    () => {
      h.mockStatResource = {
        value: null,
        loading: true,
        error: null,
        retry: vi.fn(),
      }
      h.mockEntriesResource = {
        value: null,
        loading: true,
        error: null,
        retry: vi.fn(),
      }
    },
    'Stage: reading path metadata',
  ],
  [
    'readdir',
    () => {
      h.mockEntriesResource = {
        value: null,
        loading: true,
        error: null,
        retry: vi.fn(),
      }
    },
    'Stage: reading directory entries',
  ],
]

describe('UnixFSBrowser drag gating', () => {
  afterEach(() => {
    cleanup()
  })

  beforeEach(() => {
    setUserAgent(
      'Mozilla/5.0 AppleWebKit/537.36 Chrome/124.0.0.0 Safari/537.36',
    )
    h.mockAddFiles.mockReset()
    h.mockExtractNativeUploadSelection.mockReset()
    h.mockDownloadUnixFSSelection.mockReset()
    h.mockLookup.mockReset()
    h.mockMkdirAll.mockReset()
    h.mockMknod.mockReset()
    h.mockRootHandleResource = null
    h.mockPathHandleResource = null
    h.mockEntriesResource = null
    h.mockStatResource = null
    h.mockRename.mockReset()
    h.mockRootClone.mockReset()
    h.mockRootLookupPath.mockReset()
    h.mockSyncStatus = {
      packRangeLabel: '2 / 128 KiB',
      packIndexTailLabel: '1 / 4 KiB',
      packLookupLabel: '1 opened / 2 candidates',
      packIndexCacheLabel: '0 hits / 1 misses',
    }
    h.registeredCommands.clear()
    h.latestFileListProps = null
    h.latestContextMenuProps = null
    h.latestMoveDialogProps = null
    h.latestFileViewerProps = null
    h.mockStat = {
      info: { isDir: true },
      mimeType: 'inode/directory',
    }
    h.mockFileEntries = [
      { id: 'docs', name: 'docs', isDir: true },
      { id: 'file', name: 'file.txt', isDir: false },
      { id: 'logo', name: 'logo.png', isDir: false },
    ]

    h.mockLookup.mockResolvedValue(buildDisposableHandle({ id: 77 }))
    h.mockRootClone.mockResolvedValue(
      buildDisposableHandle({
        rename: h.mockRename,
      }),
    )
    h.mockRootLookupPath.mockResolvedValue({
      handle: buildDisposableHandle({
        id: 77,
        rename: h.mockRename,
      }),
    })
    h.mockDownloadUnixFSSelection.mockResolvedValue(undefined)
    h.mockMkdirAll.mockResolvedValue(undefined)
    h.mockMknod.mockResolvedValue(undefined)
    h.mockExtractNativeUploadSelection.mockResolvedValue({
      files: [new File(['hello'], 'hello.txt')],
      directories: [],
    })
  })

  it('confirms the top-bar new folder input on Return', async () => {
    const user = userEvent.setup()
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    await user.click(screen.getByTitle('New folder'))
    const input = screen.getByPlaceholderText('Folder name')

    await user.type(input, 'Projects{Enter}')

    await waitFor(() => {
      expect(h.mockMkdirAll).toHaveBeenCalledWith(['Projects'])
    })
  })

  it('ignores internal app drags', () => {
    const { container } = render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/"
        worldState={buildResource({} as IWorldState)}
      />,
    )
    const dragSurface = container.querySelector('.bg-file-back')
    if (!dragSurface) {
      throw new Error('drag surface not found')
    }

    const dataTransfer = createInternalAppDragDataTransfer()

    expect(fireEvent.dragOver(dragSurface, { dataTransfer })).toBe(true)
    expect(screen.queryByText('Drop files to upload')).toBeNull()

    expect(fireEvent.drop(dragSurface, { dataTransfer })).toBe(true)
    expect(h.mockAddFiles).not.toHaveBeenCalled()
  })

  it('ignores internal tab drags', () => {
    const { container } = render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/"
        worldState={buildResource({} as IWorldState)}
      />,
    )
    const dragSurface = container.querySelector('.bg-file-back')
    if (!dragSurface) {
      throw new Error('drag surface not found')
    }

    const dataTransfer = createInternalTabDragDataTransfer()

    expect(fireEvent.dragOver(dragSurface, { dataTransfer })).toBe(true)
    expect(screen.queryByText('Drop files to upload')).toBeNull()

    expect(fireEvent.drop(dragSurface, { dataTransfer })).toBe(true)
    expect(h.mockAddFiles).not.toHaveBeenCalled()
  })

  it('accepts native file drags and uploads dropped files', async () => {
    const { container } = render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/"
        worldState={buildResource({} as IWorldState)}
      />,
    )
    const dragSurface = container.querySelector('.bg-file-back')
    if (!dragSurface) {
      throw new Error('drag surface not found')
    }

    const dataTransfer = createNativeFileDataTransfer()

    expect(fireEvent.dragOver(dragSurface, { dataTransfer })).toBe(false)
    expect(screen.getByText('Drop files to upload')).toBeTruthy()

    expect(fireEvent.drop(dragSurface, { dataTransfer })).toBe(false)
    await waitFor(() => {
      expect(h.mockExtractNativeUploadSelection).toHaveBeenCalledTimes(1)
      expect(h.mockAddFiles).toHaveBeenCalledWith(
        expect.arrayContaining([
          expect.objectContaining({ name: 'hello.txt' }),
        ]),
        [],
      )
    })
  })

  it('forwards explicit dropped directories to the upload manager', async () => {
    h.mockExtractNativeUploadSelection.mockResolvedValue({
      files: [new File(['hello'], 'nested/child.txt')],
      directories: ['nested', 'nested/empty'],
    })

    const { container } = render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/"
        worldState={buildResource({} as IWorldState)}
      />,
    )
    const dragSurface = container.querySelector('.bg-file-back')
    if (!dragSurface) {
      throw new Error('drag surface not found')
    }

    const dataTransfer = createNativeFileDataTransfer()
    expect(fireEvent.drop(dragSurface, { dataTransfer })).toBe(false)

    await waitFor(() => {
      expect(h.mockAddFiles).toHaveBeenCalledWith(expect.any(Array), [
        'nested',
        'nested/empty',
      ])
    })
  })

  it('moves same-unixfs drags onto folder rows', async () => {
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/"
        worldState={buildResource({} as IWorldState)}
      />,
    )
    const folderEntry = screen.getByTestId('file-entry-docs')
    const dataTransfer = createUnixFSEntryAppDragDataTransfer({})

    expect(fireEvent.dragOver(folderEntry, { dataTransfer })).toBe(false)
    expect(folderEntry.getAttribute('data-drop-active')).toBe('true')

    expect(fireEvent.drop(folderEntry, { dataTransfer })).toBe(true)

    await waitFor(() => {
      expect(h.mockRootClone).toHaveBeenCalledOnce()
      expect(h.mockRootLookupPath).toHaveBeenCalledWith('docs', undefined)
      expect(h.mockRename).toHaveBeenCalledWith(
        'file.txt',
        'file.txt',
        77,
        undefined,
      )
    })
  })

  it('keeps the toolbar visible during loading transitions without reusable entries', () => {
    h.mockEntriesResource = {
      value: null,
      loading: true,
      error: null,
      retry: vi.fn(),
    }
    h.mockStatResource = {
      value: null,
      loading: true,
      error: null,
      retry: vi.fn(),
    }

    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/test"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    expect(screen.getByText('Toolbar')).toBeTruthy()
    expect(screen.getByText('Loading files')).toBeTruthy()
    expect(
      screen.getByText('Reading the entry metadata. Path: /test'),
    ).toBeTruthy()
    expect(screen.getByText('Stage: reading path metadata')).toBeTruthy()
    expect(
      screen.getByText('Pack reads: ranges 2 / 128 KiB; index tail 1 / 4 KiB'),
    ).toBeTruthy()
    expect(
      screen.getByText(
        'Lookup: 1 opened / 2 candidates; cache 0 hits / 1 misses',
      ),
    ).toBeTruthy()
  })

  it.each(unixFSLoadingStageCases)(
    'renders %s loading diagnostics',
    (_, setup, expectedStage) => {
      setup()

      render(
        <UnixFSBrowser
          unixfsId="files"
          basePath="/"
          currentPath="/test"
          worldState={buildResource({} as IWorldState)}
        />,
      )

      expect(screen.getByTestId('unixfs-loading-diagnostics')).toBeTruthy()
      expect(screen.getByText(expectedStage)).toBeTruthy()
    },
  )

  it('rejects cross-root folder drags without activating move affordances', async () => {
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/"
        worldState={buildResource({} as IWorldState)}
      />,
    )
    const folderEntry = screen.getByTestId('file-entry-docs')
    const dataTransfer = createUnixFSEntryAppDragDataTransfer({
      unixfsId: 'unixfs/other',
    })

    expect(fireEvent.dragOver(folderEntry, { dataTransfer })).toBe(true)
    expect(folderEntry.getAttribute('data-drop-active')).toBe('false')

    expect(fireEvent.drop(folderEntry, { dataTransfer })).toBe(true)
    await waitFor(() => {
      expect(h.mockRename).not.toHaveBeenCalled()
    })
  })

  it('enables download command for a single directory selection', async () => {
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    act(() => {
      setSelection(['docs'])
    })

    await waitFor(() => {
      expect(h.registeredCommands.get('spacewave.file.download')?.enabled).toBe(
        true,
      )
    })
  })

  it('downloads the whole selection when the command is triggered for multi-select', async () => {
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/nested"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    act(() => {
      setSelection(['docs', 'file'])
    })

    act(() => {
      triggerDownloadCommand()
    })

    await waitFor(() => {
      const lastCall = h.mockDownloadUnixFSSelection.mock.calls.at(-1) as
        | [{ currentPath: string; entries: FileEntry[] }]
        | undefined
      expect(lastCall?.[0].currentPath).toBe('/nested')
      expect(lastCall?.[0].entries.map((entry) => entry.id)).toEqual([
        'docs',
        'file',
      ])
    })
  })

  it('targets the whole selection when right-clicking an already-selected row', () => {
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    act(() => {
      setSelection(['docs', 'file'])
    })

    act(() => {
      triggerRowContextMenu(h.mockFileEntries[0], 5, 7)
    })

    expect(getContextMenuActionEntryIds()).toEqual(['docs', 'file'])
    expect(getContextMenuMoveItemPaths()).toEqual(['/docs', '/file.txt'])
  })

  it('targets only the clicked row when right-clicking an unselected row', () => {
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    act(() => {
      setSelection(['docs', 'file'])
    })

    act(() => {
      triggerRowContextMenu(h.mockFileEntries[2], 9, 11)
    })

    expect(getContextMenuActionEntryIds()).toEqual(['logo'])
    expect(getContextMenuMoveItemPaths()).toEqual(['/logo.png'])
  })

  it('builds a multi-item drag envelope when dragging a selected row from a multi-selection', () => {
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    act(() => {
      setSelection(['docs', 'file'])
    })

    const envelope = getLatestDragEnvelope(h.mockFileEntries[1], [
      'docs',
      'file',
    ])
    expect(envelope?.items.map((item) => item.id)).toEqual(['docs', 'file'])
    expect(
      envelope?.items.map((item) => item.capabilities.map((cap) => cap.kind)),
    ).toEqual([
      ['openable', 'movable'],
      ['openable', 'movable'],
    ])
  })

  it('keeps unselected row drags single-item even when other rows are selected', () => {
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    act(() => {
      setSelection(['docs', 'file'])
    })

    const envelope = getLatestDragEnvelope(h.mockFileEntries[2], [
      'docs',
      'file',
    ])
    expect(envelope?.items.map((item) => item.id)).toEqual(['logo'])
  })

  it('builds a direct desktop download target for a file row', () => {
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/nested"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    const target = getLatestDownloadDragTarget(h.mockFileEntries[1])

    expect(target).toEqual({
      mimeType: 'application/octet-stream',
      filename: 'file.txt',
      url: '/p/spacewave-core/fs/u/1/so/space-test/-/files/-/nested/file.txt',
    })
  })

  it('builds a zip desktop download target for a directory row', () => {
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/nested"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    const target = getLatestDownloadDragTarget(h.mockFileEntries[0])

    expect(target).toEqual({
      mimeType: 'application/zip',
      filename: 'docs.zip',
      url: '/p/spacewave-core/export/u/1/so/space-test/-/files/-/nested/docs',
    })
  })

  it('builds a multi-entry desktop download target when dragging a selected row', () => {
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/nested"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    act(() => {
      setSelection(['docs', 'file'])
    })

    const target = getLatestDownloadDragTarget(h.mockFileEntries[1], [
      'docs',
      'file',
    ])

    expect(target?.mimeType).toBe('application/zip')
    expect(target?.filename).toBe('selection.zip')
    expect(target?.url).toMatch(
      /^\/p\/spacewave-core\/export-batch\/u\/1\/so\/space-test\/-\/files\/-\/nested\/.+\/selection\.zip$/,
    )
  })

  it('keeps unselected row desktop drags single-entry when another selection exists', () => {
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/nested"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    act(() => {
      setSelection(['docs', 'file'])
    })

    const target = getLatestDownloadDragTarget(h.mockFileEntries[2], [
      'docs',
      'file',
    ])

    expect(target).toEqual({
      mimeType: 'application/octet-stream',
      filename: 'logo.png',
      url: '/p/spacewave-core/fs/u/1/so/space-test/-/files/-/nested/logo.png',
    })
  })

  it('disables desktop download targets for Firefox while keeping app drag envelopes', () => {
    setUserAgent('Mozilla/5.0 Gecko/20100101 Firefox/124.0')
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/nested"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    expect(getLatestDownloadDragTarget(h.mockFileEntries[1])).toBeNull()
    expect(getLatestDragEnvelope(h.mockFileEntries[1])?.items).toHaveLength(1)
  })

  it('passes an inline raw file url to the file viewer for image files', () => {
    h.mockStat = {
      info: { isDir: false },
      mimeType: 'image/png',
    }

    render(
      <UnixFSBrowser
        unixfsId="docs/demo"
        basePath="/"
        currentPath="/nested/logo.png"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    expect(h.latestFileViewerProps?.path).toBe('/nested/logo.png')
    expect(h.latestFileViewerProps?.stat.mimeType).toBe('image/png')
    expect(h.latestFileViewerProps?.inlineFileURL).toBe(
      '/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/logo.png?inline=1',
    )
  })

  it('prefers the UnixfsObjectInfo mime type override when provided', () => {
    h.mockStat = {
      info: { isDir: false },
      mimeType: 'application/octet-stream',
    }

    render(
      <UnixFSBrowser
        unixfsId="docs/demo"
        basePath="/"
        currentPath="/nested/demo-video"
        mimeTypeOverride="video/mp4"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    expect(h.latestFileViewerProps?.stat.mimeType).toBe('video/mp4')
  })

  it('opens the move dialog from the context menu for the targeted selection', () => {
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    act(() => {
      setSelection(['docs', 'file'])
    })

    act(() => {
      triggerRowContextMenu(h.mockFileEntries[0], 5, 7)
    })

    act(() => {
      triggerContextMenuMove()
    })

    expect(h.latestMoveDialogProps?.moveItems.map((item) => item.path)).toEqual(
      ['/docs', '/file.txt'],
    )
  })

  it('executes modal moves through the shared move executor', async () => {
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    act(() => {
      setSelection(['docs', 'file'])
    })

    act(() => {
      triggerRowContextMenu(h.mockFileEntries[0], 5, 7)
    })

    act(() => {
      triggerContextMenuMove()
    })

    await act(async () => {
      await h.latestMoveDialogProps?.onConfirm?.('/archive')
    })

    expect(h.mockRootClone).toHaveBeenCalledOnce()
    expect(h.mockRootLookupPath).toHaveBeenCalledWith('archive', undefined)
    expect(h.mockRename.mock.calls).toEqual([
      ['docs', 'docs', 77, undefined],
      ['file.txt', 'file.txt', 77, undefined],
    ])
  })

  it('moves a multi-selection together onto a folder row', async () => {
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    act(() => {
      setSelection(['file', 'logo'])
    })

    const envelope = getLatestDragEnvelope(h.mockFileEntries[1], [
      'file',
      'logo',
    ])
    if (!envelope) {
      throw new Error('expected drag envelope')
    }

    const folderEntry = screen.getByTestId('file-entry-docs')
    const dataTransfer = createAppDragDataTransferFromEnvelope(envelope)

    expect(fireEvent.dragOver(folderEntry, { dataTransfer })).toBe(false)
    expect(folderEntry.getAttribute('data-drop-active')).toBe('true')

    expect(fireEvent.drop(folderEntry, { dataTransfer })).toBe(true)

    await waitFor(() => {
      expect(h.mockRename.mock.calls).toEqual([
        ['file.txt', 'file.txt', 77, undefined],
        ['logo.png', 'logo.png', 77, undefined],
      ])
    })
  })

  it('rejects invalid self or descendant multi-selection drops without partial moves', async () => {
    render(
      <UnixFSBrowser
        unixfsId="files"
        basePath="/"
        currentPath="/"
        worldState={buildResource({} as IWorldState)}
      />,
    )

    act(() => {
      setSelection(['docs', 'file'])
    })

    const envelope = getLatestDragEnvelope(h.mockFileEntries[0], [
      'docs',
      'file',
    ])
    if (!envelope) {
      throw new Error('expected drag envelope')
    }

    const folderEntry = screen.getByTestId('file-entry-docs')
    const dataTransfer = createAppDragDataTransferFromEnvelope(envelope)

    expect(fireEvent.dragOver(folderEntry, { dataTransfer })).toBe(true)
    expect(folderEntry.getAttribute('data-drop-active')).toBe('false')

    expect(fireEvent.drop(folderEntry, { dataTransfer })).toBe(true)
    await waitFor(() => {
      expect(h.mockRename).not.toHaveBeenCalled()
    })
  })
})
