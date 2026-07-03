import { useCallback, useMemo, useState } from 'react'

import type { NotebookSource } from './proto/notebook.pb.js'
import type { Frontmatter } from './frontmatter.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import {
  useUnixFSRootHandle,
  useUnixFSHandle,
  useUnixFSHandleEntries,
} from '@s4wave/web/hooks/useUnixFSHandle.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  LuChevronLeft,
  LuFile,
  LuFolder,
  LuFolderPlus,
  LuPenLine,
  LuPlus,
  LuSearch,
  LuTrash2,
  LuX,
} from 'react-icons/lu'

import {
  getFrontmatterTags,
  normalizeFrontmatterStatus,
  parseNote,
} from './frontmatter.js'
import { ConfirmActionDialog, TextInputDialog } from './NoteDialogs.js'
import { readFileText } from './read-file.js'
import { createTextFile } from './write-file.js'
import {
  createNoteTemplate,
  getNoteFileFormat,
  nextUntitledNoteName,
  normalizeNoteRename,
  noteTitleFromContent,
  stripNoteFileExtension,
  type NoteFileFormat,
} from './note-files.js'

const defaultNoteFormats: NoteFileFormat[] = ['markdown', 'org']

interface NoteListEntry {
  name: string
  title: string
  frontmatter: Frontmatter
  tags: string[]
  format: NoteFileFormat
}

interface NoteListProps {
  source: NotebookSource | undefined
  worldState: Resource<IWorldState>
  selectedNote: string
  currentPath?: string
  onSelectNote: (path: string) => void
  onChangePath?: (path: string) => void
  onNoteRenamed?: (prevPath: string, nextPath: string) => void
  onNoteDeleted?: (path: string) => void
  filterTag?: string
  filterStatus?: string
  onFilterTagChange?: (tag: string | undefined) => void
  onFilterStatusChange?: (status: string | undefined) => void
  onCreateNote?: () => void
  allowedFormats?: NoteFileFormat[]
  renderEntryExtra?: (name: string) => React.ReactNode
}

// NoteList lists notebook directories and note files for the selected source.
function NoteList({
  source,
  worldState,
  selectedNote,
  currentPath = '',
  onSelectNote,
  onChangePath,
  onNoteRenamed,
  onNoteDeleted,
  filterTag,
  filterStatus,
  onFilterTagChange,
  onFilterStatusChange,
  onCreateNote,
  allowedFormats,
  renderEntryExtra,
}: NoteListProps) {
  const [searchQuery, setSearchQuery] = useState('')
  const [folderDialogOpen, setFolderDialogOpen] = useState(false)
  const [renameTarget, setRenameTarget] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const sourceRef = source?.ref

  const parsed = useMemo(() => {
    if (!sourceRef) return null
    return parseObjectUri(sourceRef)
  }, [sourceRef])

  const objectKey = parsed?.objectKey ?? ''
  const subpath = parsed?.path ?? ''
  const listPath = useMemo(
    () => [subpath, currentPath].filter(Boolean).join('/'),
    [subpath, currentPath],
  )

  const rootHandle = useUnixFSRootHandle(worldState, objectKey)
  const pathHandle = useUnixFSHandle(rootHandle, listPath)
  const entriesResource = useUnixFSHandleEntries(pathHandle)

  const dirEntries = useMemo(() => {
    if (!entriesResource.value) return []
    return entriesResource.value.filter((entry) => entry.isDir)
  }, [entriesResource.value])

  const fileEntries = useMemo(() => {
    if (!entriesResource.value) return []
    return entriesResource.value.filter((entry) => {
      const format = getNoteFileFormat(entry.name)
      return (
        !entry.isDir &&
        format !== null &&
        (allowedFormats ?? defaultNoteFormats).includes(format)
      )
    })
  }, [entriesResource.value, allowedFormats])

  const noteEntries = useResource(
    pathHandle,
    async (handle, signal) => {
      if (!handle || fileEntries.length === 0) return []

      const entries: NoteListEntry[] = []
      for (const entry of fileEntries) {
        if (signal.aborted) return entries

        const child = await handle.lookup(entry.name, signal)
        const text = await readFileText(child, signal).finally(() =>
          child.release(),
        )
        const format = getNoteFileFormat(entry.name)
        if (!format) continue

        const note = format === 'markdown' ? parseNote(text) : null
        entries.push({
          name: entry.name,
          title:
            format === 'markdown' &&
            typeof note?.frontmatter.title === 'string' &&
            note.frontmatter.title.trim()
              ? note.frontmatter.title.trim()
              : noteTitleFromContent(entry.name, text),
          frontmatter: note?.frontmatter ?? {},
          tags: note ? getFrontmatterTags(note.frontmatter) : [],
          status: note
            ? normalizeFrontmatterStatus(note.frontmatter.status)
            : undefined,
          format,
        })
      }

      return entries
    },
    [fileEntries],
  )

  const filteredDirEntries = useMemo(() => {
    if (filterTag || filterStatus) return []

    let entries = dirEntries
    if (searchQuery) {
      const lower = searchQuery.toLowerCase()
      entries = entries.filter((entry) =>
        entry.name.toLowerCase().includes(lower),
      )
    }
    return entries
  }, [dirEntries, searchQuery, filterTag, filterStatus])

  const filteredNoteEntries = useMemo(() => {
    let entries = noteEntries.value ?? []
    if (searchQuery) {
      const lower = searchQuery.toLowerCase()
      entries = entries.filter(
        (entry) =>
          entry.name.toLowerCase().includes(lower) ||
          entry.title.toLowerCase().includes(lower),
      )
    }
    if (filterTag) {
      const lower = filterTag.toLowerCase()
      entries = entries.filter((entry) =>
        entry.tags.some((tag) => tag.toLowerCase() === lower),
      )
    }
    if (filterStatus) {
      const normalized = normalizeFrontmatterStatus(filterStatus)
      entries = entries.filter((entry) => entry.status === normalized)
    }
    return entries
  }, [noteEntries.value, searchQuery, filterTag, filterStatus])

  const handleCreateNoteDefault = useCallback(
    async (format: NoteFileFormat = 'markdown') => {
      const handle = pathHandle.value
      if (!handle) return

      const existing = new Set((entriesResource.value ?? []).map((e) => e.name))
      const name = nextUntitledNoteName(existing, format)
      const template = createNoteTemplate(name, format)
      await createTextFile(handle, name, template)
      onSelectNote(joinNotePath(currentPath, name))
    },
    [pathHandle.value, entriesResource.value, currentPath, onSelectNote],
  )

  const handleCreateFolder = useCallback(() => {
    setFolderDialogOpen(true)
  }, [])

  const handleConfirmCreateFolder = useCallback(
    async (name: string) => {
      const handle = pathHandle.value
      if (!handle) return

      const parts = name.split('/').flatMap((part) => {
        const trimmed = part.trim()
        return trimmed ? [trimmed] : []
      })
      if (parts.length === 0) return

      await handle.mkdirAll(parts)
      setFolderDialogOpen(false)
    },
    [pathHandle.value],
  )

  const handleRenameNote = useCallback((name: string) => {
    setRenameTarget(name)
  }, [])

  const handleConfirmRenameNote = useCallback(
    async (name: string, nextTitle: string) => {
      const handle = pathHandle.value
      if (!handle) return

      const nextName = normalizeNoteRename(name, nextTitle)
      if (!nextName) return
      if (nextName === name) return

      await handle.rename(name, nextName)
      onNoteRenamed?.(
        joinNotePath(currentPath, name),
        joinNotePath(currentPath, nextName),
      )
      setRenameTarget(null)
    },
    [pathHandle.value, currentPath, onNoteRenamed],
  )

  const handleDeleteNote = useCallback((name: string) => {
    setDeleteTarget(name)
  }, [])

  const handleConfirmDeleteNote = useCallback(async () => {
    const handle = pathHandle.value
    if (!handle || !deleteTarget) return

    await handle.remove([deleteTarget])
    onNoteDeleted?.(joinNotePath(currentPath, deleteTarget))
    setDeleteTarget(null)
  }, [pathHandle.value, currentPath, deleteTarget, onNoteDeleted])

  const renameDefaultValue = renameTarget
    ? stripNoteFileExtension(renameTarget)
    : ''
  const closeRenameDialog = useCallback((open: boolean) => {
    if (!open) setRenameTarget(null)
  }, [])
  const closeDeleteDialog = useCallback((open: boolean) => {
    if (!open) setDeleteTarget(null)
  }, [])

  const handleCreateNote = onCreateNote ?? (() => handleCreateNoteDefault())
  const canCreateOrg = (allowedFormats ?? defaultNoteFormats).includes('org')
  const hasFilter = !!filterTag || !!filterStatus
  const showEmptyState =
    filteredDirEntries.length === 0 && filteredNoteEntries.length === 0
  const isEmptyDirectory =
    !hasFilter &&
    !searchQuery &&
    dirEntries.length === 0 &&
    fileEntries.length === 0

  if (!source) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center text-xs">
        Select a source
      </div>
    )
  }

  if (!objectKey) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center p-4 text-center text-xs">
        Invalid source ref
      </div>
    )
  }

  if (entriesResource.loading) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center text-xs">
        Loading…
      </div>
    )
  }

  if (entriesResource.error) {
    return (
      <div className="text-destructive flex h-full items-center justify-center p-4 text-center text-xs">
        {entriesResource.error.message}
      </div>
    )
  }

  if (noteEntries.error) {
    return (
      <div className="text-destructive flex h-full items-center justify-center p-4 text-center text-xs">
        {noteEntries.error.message}
      </div>
    )
  }

  if (noteEntries.loading && fileEntries.length > 0) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center text-xs">
        Loading…
      </div>
    )
  }

  return (
    <>
      <div
        className="flex h-full flex-col overflow-y-auto"
        data-testid="notes-note-list"
      >
        <div className="border-border flex items-center gap-1 border-b px-2 py-1.5">
          <div className="bg-muted flex flex-1 items-center gap-1.5 rounded px-2 py-1">
            <LuSearch className="text-muted-foreground size-3 shrink-0" />
            <input
              type="text"
              placeholder="Search notes..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="text-foreground placeholder:text-muted-foreground w-full border-none bg-transparent text-xs outline-none"
            />
          </div>
          <button
            type="button"
            className="text-foreground-alt hover:bg-list-hover-background hover:text-foreground flex items-center justify-center rounded p-1.5"
            onClick={() => handleCreateFolder()}
            title="New folder"
          >
            <LuFolderPlus className="size-3.5" />
          </button>
          <button
            type="button"
            className="text-foreground-alt hover:bg-list-hover-background hover:text-foreground flex items-center justify-center rounded p-1.5"
            onClick={() => void handleCreateNote()}
            title="New note"
          >
            <LuPlus className="size-3.5" />
          </button>
          {canCreateOrg && (
            <button
              type="button"
              className="text-foreground-alt hover:bg-list-hover-background hover:text-foreground flex items-center justify-center rounded px-1.5 py-1 text-[10px] font-medium"
              onClick={() => void handleCreateNoteDefault('org')}
              title="New Org note"
            >
              Org
            </button>
          )}
        </div>
        {currentPath && (
          <div className="border-border flex items-center gap-1 border-b px-2 py-1 text-xs">
            <button
              type="button"
              className="text-foreground-alt hover:bg-list-hover-background hover:text-foreground rounded p-1"
              onClick={() => onChangePath?.(getParentPath(currentPath))}
              title="Up one level"
            >
              <LuChevronLeft className="size-3" />
            </button>
            <span className="text-muted-foreground truncate">
              /{currentPath}
            </span>
          </div>
        )}
        {hasFilter && (
          <div className="bg-brand/5 border-border flex flex-wrap items-center gap-2 border-b px-3 py-1 text-xs">
            <span className="text-muted-foreground">Filtering:</span>
            {filterTag && (
              <button
                type="button"
                className="bg-brand/10 text-brand inline-flex items-center gap-1 rounded-full px-2 py-0.5 font-medium"
                onClick={() => onFilterTagChange?.(undefined)}
                title="Clear tag filter"
              >
                {filterTag}
                <LuX className="size-2.5" />
              </button>
            )}
            {filterStatus && (
              <button
                type="button"
                className="bg-muted text-foreground-alt inline-flex items-center gap-1 rounded-full px-2 py-0.5 font-medium"
                onClick={() => onFilterStatusChange?.(undefined)}
                title="Clear status filter"
              >
                {filterStatus}
                <LuX className="size-2.5" />
              </button>
            )}
          </div>
        )}
        <div className="flex-1 overflow-y-auto">
          {showEmptyState ? (
            <div className="text-muted-foreground flex flex-col items-center justify-center gap-3 p-6 text-center">
              {isEmptyDirectory ? (
                <>
                  <span className="text-xs">No notes yet</span>
                  <button
                    type="button"
                    className="bg-brand text-brand-foreground rounded-md px-3 py-1.5 text-xs font-medium hover:opacity-90"
                    onClick={handleCreateNote}
                  >
                    Create your first note
                  </button>
                </>
              ) : (
                <span className="text-xs">No matching notes</span>
              )}
            </div>
          ) : (
            <>
              {filteredDirEntries.map((entry) => (
                <button
                  key={entry.name}
                  type="button"
                  className="hover:bg-list-hover-background flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs"
                  onClick={() =>
                    onChangePath?.(joinNotePath(currentPath, entry.name))
                  }
                >
                  <LuFolder className="size-3 shrink-0" />
                  <span className="min-w-0 flex-1 truncate">{entry.name}</span>
                </button>
              ))}
              {filteredNoteEntries.map((entry) => {
                const notePath = joinNotePath(currentPath, entry.name)
                const selected = selectedNote === notePath
                return (
                  <div
                    key={notePath}
                    className={cn(
                      'flex items-center gap-1 pr-1',
                      selected &&
                        'bg-list-active-selection-background text-list-active-selection-foreground',
                    )}
                  >
                    <button
                      type="button"
                      data-testid="notes-note-row"
                      data-note-path={notePath}
                      className="hover:bg-list-hover-background flex min-w-0 flex-1 items-center gap-2 px-3 py-1.5 text-left text-xs"
                      onClick={() => onSelectNote(notePath)}
                    >
                      <LuFile className="size-3 shrink-0" />
                      <span className="min-w-0 flex-1 truncate">
                        {entry.title}
                      </span>
                      {renderEntryExtra?.(notePath)}
                    </button>
                    <button
                      type="button"
                      className="text-foreground-alt hover:bg-list-hover-background hover:text-foreground rounded p-1"
                      onClick={() => handleRenameNote(entry.name)}
                      title="Rename note"
                    >
                      <LuPenLine className="size-3" />
                    </button>
                    <button
                      type="button"
                      className="text-foreground-alt hover:bg-list-hover-background hover:text-destructive rounded p-1"
                      onClick={() => handleDeleteNote(entry.name)}
                      title="Delete note"
                    >
                      <LuTrash2 className="size-3" />
                    </button>
                  </div>
                )
              })}
            </>
          )}
        </div>
      </div>
      <TextInputDialog
        open={folderDialogOpen}
        title="New folder"
        label="Folder name"
        placeholder="projects/client-a"
        confirmLabel="Create folder"
        requireValue
        onOpenChange={setFolderDialogOpen}
        onConfirm={(value) => void handleConfirmCreateFolder(value)}
      />
      <TextInputDialog
        open={renameTarget !== null}
        title="Rename note"
        label="Note name"
        defaultValue={renameDefaultValue}
        confirmLabel="Rename"
        requireValue
        onOpenChange={closeRenameDialog}
        onConfirm={(value) =>
          renameTarget && void handleConfirmRenameNote(renameTarget, value)
        }
      />
      <ConfirmActionDialog
        open={deleteTarget !== null}
        title="Delete note"
        description={
          deleteTarget ? `Delete "${deleteTarget}"?` : 'Delete this note?'
        }
        confirmLabel="Delete"
        onOpenChange={closeDeleteDialog}
        onConfirm={() => void handleConfirmDeleteNote()}
      />
    </>
  )
}

function joinNotePath(parent: string, name: string): string {
  return parent ? `${parent}/${name}` : name
}

function getParentPath(path: string): string {
  const parts = path.split('/').filter(Boolean)
  parts.pop()
  return parts.join('/')
}

export default NoteList
