import { useCallback, useMemo, useState } from 'react'

import type { NotebookSource } from './proto/notebook.pb.js'
import type { Frontmatter } from './frontmatter.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import { MknodType } from '@s4wave/sdk/unixfs/index.js'
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

interface NoteListEntry {
  name: string
  title: string
  frontmatter: Frontmatter
  tags: string[]
  status: string | undefined
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
  renderEntryExtra?: (name: string) => React.ReactNode
}

// NoteList lists notebook directories and markdown files for the selected source.
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
  renderEntryExtra,
}: NoteListProps) {
  const [searchQuery, setSearchQuery] = useState('')

  const parsed = useMemo(() => {
    if (!source?.ref) return null
    return parseObjectUri(source.ref)
  }, [source?.ref])

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

  const mdEntries = useMemo(() => {
    if (!entriesResource.value) return []
    return entriesResource.value.filter(
      (entry) => !entry.isDir && entry.name.endsWith('.md'),
    )
  }, [entriesResource.value])

  const noteEntries = useResource(
    pathHandle,
    async (handle, signal) => {
      if (!handle || mdEntries.length === 0) return []

      const entries: NoteListEntry[] = []
      for (const entry of mdEntries) {
        if (signal.aborted) return entries

        const child = await handle.lookup(entry.name, signal)
        const result = await child
          .readAt(0n, 0n, signal)
          .finally(() => child.release())
        const text = new TextDecoder().decode(result.data)
        const note = parseNote(text)
        entries.push({
          name: entry.name,
          title:
            typeof note.frontmatter.title === 'string' &&
            note.frontmatter.title.trim()
              ? note.frontmatter.title.trim()
              : entry.name.replace(/\.md$/, ''),
          frontmatter: note.frontmatter,
          tags: getFrontmatterTags(note.frontmatter),
          status: normalizeFrontmatterStatus(note.frontmatter.status),
        })
      }

      return entries
    },
    [mdEntries],
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
      entries = entries.filter((entry) =>
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

  const handleCreateNoteDefault = useCallback(async () => {
    const handle = pathHandle.value
    if (!handle) return

    const existing = new Set((entriesResource.value ?? []).map((e) => e.name))
    let name = 'untitled.md'
    let counter = 1
    while (existing.has(name)) {
      name = `untitled-${counter}.md`
      counter++
    }

    await handle.mknod([name], MknodType.FILE)
    const child = await handle.lookup(name)
    const title = name.replace(/\.md$/, '')
    const template = `---\ncreated: ${new Date().toISOString().slice(0, 10)}\ntags: []\n---\n\n# ${title}\n\n`
    const encoded = new TextEncoder().encode(template)
    await child.writeAt(0n, encoded)
    child.release()
    onSelectNote(joinNotePath(currentPath, name))
  }, [pathHandle.value, entriesResource.value, currentPath, onSelectNote])

  const handleCreateFolder = useCallback(async () => {
    const prompt = globalThis.prompt
    const handle = pathHandle.value
    if (!prompt || !handle) return

    const name = prompt('Folder name')?.trim() ?? ''
    const parts = name.split('/').map((part) => part.trim()).filter(Boolean)
    if (parts.length === 0) return

    await handle.mkdirAll(parts)
  }, [pathHandle.value])

  const handleRenameNote = useCallback(
    async (name: string) => {
      const prompt = globalThis.prompt
      const handle = pathHandle.value
      if (!prompt || !handle) return

      let nextName =
        prompt('Rename note', name.replace(/\.md$/, ''))?.trim() ?? ''
      if (!nextName) return
      if (!nextName.endsWith('.md')) nextName += '.md'
      if (nextName === name) return

      await handle.rename(name, nextName)
      onNoteRenamed?.(
        joinNotePath(currentPath, name),
        joinNotePath(currentPath, nextName),
      )
    },
    [pathHandle.value, currentPath, onNoteRenamed],
  )

  const handleDeleteNote = useCallback(
    async (name: string) => {
      const confirm = globalThis.confirm
      const handle = pathHandle.value
      if (!handle) return
      if (confirm && !confirm('Delete this note?')) return

      await handle.remove([name])
      onNoteDeleted?.(joinNotePath(currentPath, name))
    },
    [pathHandle.value, currentPath, onNoteDeleted],
  )

  const handleCreateNote = onCreateNote ?? handleCreateNoteDefault
  const hasFilter = !!filterTag || !!filterStatus
  const showEmptyState =
    filteredDirEntries.length === 0 && filteredNoteEntries.length === 0
  const isEmptyDirectory = !hasFilter && !searchQuery && dirEntries.length === 0 && mdEntries.length === 0

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
        Loading...
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

  if (noteEntries.loading && mdEntries.length > 0) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center text-xs">
        Loading...
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col overflow-y-auto">
      <div className="flex items-center gap-1 border-b border-border px-2 py-1.5">
        <div className="flex flex-1 items-center gap-1.5 rounded bg-muted px-2 py-1">
          <LuSearch className="text-muted-foreground h-3 w-3 shrink-0" />
          <input
            type="text"
            placeholder="Search notes..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="bg-transparent text-foreground placeholder:text-muted-foreground w-full border-none text-xs outline-none"
          />
        </div>
        <button
          type="button"
          className="text-foreground-alt hover:bg-list-hover-background hover:text-foreground flex items-center justify-center rounded p-1.5"
          onClick={handleCreateFolder}
          title="New folder"
        >
          <LuFolderPlus className="h-3.5 w-3.5" />
        </button>
        <button
          type="button"
          className="text-foreground-alt hover:bg-list-hover-background hover:text-foreground flex items-center justify-center rounded p-1.5"
          onClick={handleCreateNote}
          title="New note"
        >
          <LuPlus className="h-3.5 w-3.5" />
        </button>
      </div>
      {currentPath && (
        <div className="flex items-center gap-1 border-b border-border px-2 py-1 text-xs">
          <button
            type="button"
            className="text-foreground-alt hover:bg-list-hover-background hover:text-foreground rounded p-1"
            onClick={() => onChangePath?.(getParentPath(currentPath))}
            title="Up one level"
          >
            <LuChevronLeft className="h-3 w-3" />
          </button>
          <span className="text-muted-foreground truncate">/{currentPath}</span>
        </div>
      )}
      {hasFilter && (
        <div className="bg-brand/5 flex flex-wrap items-center gap-2 border-b border-border px-3 py-1 text-xs">
          <span className="text-muted-foreground">Filtering:</span>
          {filterTag && (
            <button
              type="button"
              className="bg-brand/10 text-brand inline-flex items-center gap-1 rounded-full px-2 py-0.5 font-medium"
              onClick={() => onFilterTagChange?.(undefined)}
              title="Clear tag filter"
            >
              {filterTag}
              <LuX className="h-2.5 w-2.5" />
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
              <LuX className="h-2.5 w-2.5" />
            </button>
          )}
        </div>
      )}
      <div className="flex-1 overflow-y-auto">
        {showEmptyState ?
          <div className="text-muted-foreground flex flex-col items-center justify-center gap-3 p-6 text-center">
            {isEmptyDirectory ?
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
            : <span className="text-xs">No matching notes</span>}
          </div>
        : <>
            {filteredDirEntries.map((entry) => (
              <button
                key={entry.name}
                type="button"
                className="hover:bg-list-hover-background flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs"
                onClick={() => onChangePath?.(joinNotePath(currentPath, entry.name))}
              >
                <LuFolder className="h-3 w-3 shrink-0" />
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
                    className="hover:bg-list-hover-background flex min-w-0 flex-1 items-center gap-2 px-3 py-1.5 text-left text-xs"
                    onClick={() => onSelectNote(notePath)}
                  >
                    <LuFile className="h-3 w-3 shrink-0" />
                    <span className="min-w-0 flex-1 truncate">{entry.title}</span>
                    {renderEntryExtra?.(notePath)}
                  </button>
                  <button
                    type="button"
                    className="text-foreground-alt hover:bg-list-hover-background hover:text-foreground rounded p-1"
                    onClick={() => void handleRenameNote(entry.name)}
                    title="Rename note"
                  >
                    <LuPenLine className="h-3 w-3" />
                  </button>
                  <button
                    type="button"
                    className="text-foreground-alt hover:bg-list-hover-background hover:text-destructive rounded p-1"
                    onClick={() => void handleDeleteNote(entry.name)}
                    title="Delete note"
                  >
                    <LuTrash2 className="h-3 w-3" />
                  </button>
                </div>
              )
            })}
          </>}
      </div>
    </div>
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
