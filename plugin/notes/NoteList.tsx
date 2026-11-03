import { useCallback, useMemo, useState, type ReactNode } from 'react'

import type { NotebookSource } from './proto/notebook.pb.js'
import type { Frontmatter } from './frontmatter.js'
import type { SavedView } from './saved-views.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import {
  useUnixFSRootHandle,
  useUnixFSHandle,
  useUnixFSHandleEntries,
} from '@s4wave/web/hooks/useUnixFSHandle.js'

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
} from '@s4wave/web/images/icons.js'

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

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  radii as themeRadii,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: themeColors['--color-border'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule2: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '.25rem',
    backgroundColor: themeColors['--color-muted'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': themeLayout['--spacing'],
      },
    },
  },
  rule3: {
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    width: '100%',
    minWidth: 0,
    '--tw-border-style': 'none',
    borderStyle: 'none',
    backgroundColor: '#0000',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': { color: themeColors['--color-muted-foreground'] },
  },
  rule4: {
    display: 'flex',
    width: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    height: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    padding: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 1.5)`,
      },
    },
  },
  rule5: {
    display: 'flex',
    width: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    height: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    paddingInline: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 1.5)`,
      },
    },
    paddingBlock: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': themeLayout['--spacing'],
      },
    },
  },
  rule6: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: themeColors['--color-border'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule7: { color: themeColors['--color-muted-foreground'] },
  rule8: {
    display: 'inline-flex',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    minWidth: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    alignItems: 'center',
    justifyContent: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand'],
  },
  rule9: {
    display: 'inline-flex',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    minWidth: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    alignItems: 'center',
    justifyContent: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-muted'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
  },
  rule10: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    textAlign: 'center',
    color: themeColors['--color-muted-foreground'],
  },
  rule11: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule12: {
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: themeColors['--color-brand'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand-foreground'],
    opacity: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 0.9 },
    },
  },
  rule13: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    paddingRight: themeLayout['--spacing'],
  },
  rule14: {
    backgroundColor: themeColors['--color-list-active-selection-background'],
    color: themeColors['--color-list-active-selection-foreground'],
  },
  rule15: {
    display: 'flex',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    minWidth: 0,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    paddingBlock: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 1.5)`,
      },
    },
  },
  rule16: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
  },
  rule17: {
    display: 'flex',
    width: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    height: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    padding: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': themeLayout['--spacing'],
      },
    },
  },
  rule18: {
    display: 'flex',
    width: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    height: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    padding: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': themeLayout['--spacing'],
      },
    },
  },
  rule19: {
    display: 'flex',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    paddingBlock: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 1.5)`,
      },
    },
  },
  rule20: {
    display: 'flex',
    height: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule21: { color: themeColors['--color-destructive'] },
  rule22: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
  },
  rule23: {
    display: 'flex',
    height: '100%',
    flexDirection: 'column',
    overflowY: 'auto',
  },
  rule24: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: themeColors['--color-border'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule25: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    color: themeColors['--color-muted-foreground'],
  },
  rule26: { flex: 1, overflowY: 'auto' },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    flexShrink: 0,
    color: themeColors['--color-muted-foreground'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    flexShrink: 0,
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const defaultNoteFormats: NoteFileFormat[] = ['markdown', 'org']

interface NoteListEntry {
  name: string
  title: string
  frontmatter: Frontmatter
  tags: string[]
  status?: string
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
  sort?: SavedView['sort']
  onFilterTagChange?: (tag: string | undefined) => void
  onFilterStatusChange?: (status: string | undefined) => void
  onCreateNote?: () => void
  allowedFormats?: NoteFileFormat[]
  renderEntryExtra?: (name: string) => React.ReactNode
}

// useNoteListController owns UnixFS resources, filtering, and note mutations.
function useNoteListController({
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
  sort = 'name',
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
    const formats = new Set(allowedFormats ?? defaultNoteFormats)
    return entriesResource.value.filter((entry) => {
      const format = getNoteFileFormat(entry.name)
      return !entry.isDir && format !== null && formats.has(format)
    })
  }, [entriesResource.value, allowedFormats])

  const noteEntries = useResource(
    pathHandle,
    async (handle, signal) => {
      if (!handle || fileEntries.length === 0) return []

      const entries: NoteListEntry[] = []
      for (const entry of fileEntries) {
        if (signal.aborted) return entries

        // eslint-disable-next-line react-doctor/async-await-in-loop -- note files load through one directory handle so child refs release before the next lookup.
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
    return entries.toSorted(
      (a, b) =>
        (sort === 'title'
          ? a.title.localeCompare(b.title)
          : a.name.localeCompare(b.name)) || a.name.localeCompare(b.name),
    )
  }, [noteEntries.value, searchQuery, filterTag, filterStatus, sort])

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

  return {
    canCreateOrg,
    closeDeleteDialog,
    closeRenameDialog,
    currentPath,
    deleteTarget,
    entriesResource,
    fileEntries,
    filterStatus,
    filterTag,
    sort,
    filteredDirEntries,
    filteredNoteEntries,
    folderDialogOpen,
    handleConfirmCreateFolder,
    handleConfirmDeleteNote,
    handleConfirmRenameNote,
    handleCreateFolder,
    handleCreateNote,
    handleCreateNoteDefault,
    handleDeleteNote,
    handleRenameNote,
    hasFilter,
    isEmptyDirectory,
    noteEntries,
    objectKey,
    onChangePath,
    onFilterStatusChange,
    onFilterTagChange,
    onSelectNote,
    renameDefaultValue,
    renameTarget,
    renderEntryExtra,
    searchQuery,
    selectedNote,
    setFolderDialogOpen,
    setSearchQuery,
    showEmptyState,
    source,
  }
}

type NoteListController = ReturnType<typeof useNoteListController>

// noteListNotice returns the message that replaces the list while the source
// or its entries are not ready, or null when the list can render.
function noteListNotice(
  controller: NoteListController,
): { text: string; error: boolean; padded: boolean } | null {
  const { entriesResource, fileEntries, noteEntries, objectKey, source } =
    controller
  if (!source) return { text: 'Select a source', error: false, padded: false }
  if (!objectKey) {
    return { text: 'Invalid source ref', error: false, padded: true }
  }
  if (entriesResource.loading) {
    return { text: 'Loading…', error: false, padded: false }
  }
  if (entriesResource.error) {
    return { text: entriesResource.error.message, error: true, padded: true }
  }
  if (noteEntries.error) {
    return { text: noteEntries.error.message, error: true, padded: true }
  }
  if (noteEntries.loading && fileEntries.length > 0) {
    return { text: 'Loading…', error: false, padded: false }
  }
  return null
}

// NoteListToolbar renders the note search field and the create actions.
function NoteListToolbar({
  controller,
}: {
  controller: Pick<
    NoteListController,
    | 'canCreateOrg'
    | 'handleCreateFolder'
    | 'handleCreateNote'
    | 'handleCreateNoteDefault'
    | 'searchQuery'
    | 'setSearchQuery'
  >
}) {
  const {
    canCreateOrg,
    handleCreateFolder,
    handleCreateNote,
    handleCreateNoteDefault,
    searchQuery,
    setSearchQuery,
  } = controller

  return (
    <div sx={stylexStyles2.rule1}>
      <label sx={stylexStyles2.rule2}>
        <LuSearch sx={stylexStyles.rule1} />
        <input
          type="text"
          aria-label="Search notes"
          placeholder="Search notes…"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          sx={stylexStyles2.rule3}
        />
      </label>
      <button
        type="button"
        sx={stylexStyles2.rule4}
        onClick={() => handleCreateFolder()}
        title="New folder"
      >
        <LuFolderPlus sx={stylexStyles.rule2} />
      </button>
      <button
        type="button"
        sx={stylexStyles2.rule4}
        onClick={() => void handleCreateNote()}
        title="New note"
      >
        <LuPlus sx={stylexStyles.rule2} />
      </button>
      {canCreateOrg && (
        <button
          type="button"
          sx={stylexStyles2.rule5}
          onClick={() => void handleCreateNoteDefault('org')}
          title="New Org note"
        >
          Org
        </button>
      )}
    </div>
  )
}

// NoteListFilters renders the active tag and status filters with clear actions.
function NoteListFilters({
  controller,
}: {
  controller: Pick<
    NoteListController,
    'filterStatus' | 'filterTag' | 'onFilterStatusChange' | 'onFilterTagChange'
  >
}) {
  const { filterStatus, filterTag, onFilterStatusChange, onFilterTagChange } =
    controller

  return (
    <div sx={stylexStyles2.rule6}>
      <span sx={stylexStyles2.rule7}>Filtering:</span>
      {filterTag && (
        <button
          type="button"
          sx={stylexStyles2.rule8}
          onClick={() => onFilterTagChange?.(undefined)}
          title="Clear tag filter"
        >
          {filterTag}
          <LuX sx={stylexStyles.rule3} />
        </button>
      )}
      {filterStatus && (
        <button
          type="button"
          sx={stylexStyles2.rule9}
          onClick={() => onFilterStatusChange?.(undefined)}
          title="Clear status filter"
        >
          {filterStatus}
          <LuX sx={stylexStyles.rule3} />
        </button>
      )}
    </div>
  )
}

// NoteListEmptyState explains why the list has no entries.
function NoteListEmptyState({
  isEmptyDirectory,
  onCreateNote,
}: {
  isEmptyDirectory: boolean
  onCreateNote: NoteListController['handleCreateNote']
}) {
  return (
    <div sx={stylexStyles2.rule10}>
      {isEmptyDirectory ? (
        <>
          <span sx={stylexStyles2.rule11}>No notes yet</span>
          <button
            type="button"
            sx={stylexStyles2.rule12}
            onClick={onCreateNote}
          >
            Create your first note
          </button>
        </>
      ) : (
        <span sx={stylexStyles2.rule11}>No matching notes</span>
      )}
    </div>
  )
}

// NoteListNoteRow renders one note with its rename and delete actions.
function NoteListNoteRow({
  entry,
  notePath,
  selected,
  extra,
  onSelect,
  onRename,
  onDelete,
}: {
  entry: NoteListEntry
  notePath: string
  selected: boolean
  extra: ReactNode
  onSelect: (notePath: string) => void
  onRename: (name: string) => void
  onDelete: (name: string) => void
}) {
  return (
    <div sx={[stylexStyles2.rule13, !!selected && stylexStyles2.rule14]}>
      <button
        type="button"
        data-testid="notes-note-row"
        data-note-path={notePath}
        sx={stylexStyles2.rule15}
        onClick={() => onSelect(notePath)}
      >
        <LuFile sx={stylexStyles.rule4} />
        <span sx={stylexStyles2.rule16}>{entry.title}</span>
        {extra}
      </button>
      <button
        type="button"
        sx={stylexStyles2.rule17}
        onClick={() => onRename(entry.name)}
        title="Rename note"
      >
        <LuPenLine sx={stylexStyles.rule5} />
      </button>
      <button
        type="button"
        sx={stylexStyles2.rule18}
        onClick={() => onDelete(entry.name)}
        title="Delete note"
      >
        <LuTrash2 sx={stylexStyles.rule5} />
      </button>
    </div>
  )
}

// NoteListEntries renders the folders and notes of the current directory.
function NoteListEntries({
  controller,
}: {
  controller: Pick<
    NoteListController,
    | 'currentPath'
    | 'filteredDirEntries'
    | 'filteredNoteEntries'
    | 'handleDeleteNote'
    | 'handleRenameNote'
    | 'onChangePath'
    | 'onSelectNote'
    | 'renderEntryExtra'
    | 'selectedNote'
  >
}) {
  const {
    currentPath,
    filteredDirEntries,
    filteredNoteEntries,
    handleDeleteNote,
    handleRenameNote,
    onChangePath,
    onSelectNote,
    renderEntryExtra,
    selectedNote,
  } = controller

  return (
    <>
      {filteredDirEntries.map((entry) => (
        <button
          key={entry.name}
          type="button"
          sx={stylexStyles2.rule19}
          onClick={() => onChangePath?.(joinNotePath(currentPath, entry.name))}
        >
          <LuFolder sx={stylexStyles.rule4} />
          <span sx={stylexStyles2.rule16}>{entry.name}</span>
        </button>
      ))}
      {filteredNoteEntries.map((entry) => {
        const notePath = joinNotePath(currentPath, entry.name)
        return (
          <NoteListNoteRow
            key={notePath}
            entry={entry}
            notePath={notePath}
            selected={selectedNote === notePath}
            extra={renderEntryExtra?.(notePath)}
            onSelect={onSelectNote}
            onRename={handleRenameNote}
            onDelete={handleDeleteNote}
          />
        )
      })}
    </>
  )
}

// NoteListDialogs renders the folder, rename, and delete dialogs.
function NoteListDialogs({
  controller,
}: {
  controller: Pick<
    NoteListController,
    | 'closeDeleteDialog'
    | 'closeRenameDialog'
    | 'deleteTarget'
    | 'folderDialogOpen'
    | 'handleConfirmCreateFolder'
    | 'handleConfirmDeleteNote'
    | 'handleConfirmRenameNote'
    | 'renameDefaultValue'
    | 'renameTarget'
    | 'setFolderDialogOpen'
  >
}) {
  const {
    closeDeleteDialog,
    closeRenameDialog,
    deleteTarget,
    folderDialogOpen,
    handleConfirmCreateFolder,
    handleConfirmDeleteNote,
    handleConfirmRenameNote,
    renameDefaultValue,
    renameTarget,
    setFolderDialogOpen,
  } = controller

  return (
    <>
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

// NoteListView renders note navigation and dialogs once the source is ready.
function NoteListView({ controller }: { controller: NoteListController }) {
  const { currentPath, hasFilter, isEmptyDirectory, onChangePath } = controller
  const notice = noteListNotice(controller)

  if (notice) {
    return (
      <div
        sx={[
          stylexStyles2.rule20,
          notice.error ? stylexStyles2.rule21 : stylexStyles2.rule7,
          !!notice.padded && stylexStyles2.rule22,
        ]}
      >
        {notice.text}
      </div>
    )
  }

  return (
    <>
      <div sx={stylexStyles2.rule23} data-testid="notes-note-list">
        <NoteListToolbar controller={controller} />
        {currentPath && (
          <div sx={stylexStyles2.rule24}>
            <button
              type="button"
              sx={stylexStyles2.rule17}
              onClick={() => onChangePath?.(getParentPath(currentPath))}
              title="Up one level"
            >
              <LuChevronLeft sx={stylexStyles.rule5} />
            </button>
            <span sx={stylexStyles2.rule25}>/{currentPath}</span>
          </div>
        )}
        {hasFilter && <NoteListFilters controller={controller} />}
        <div sx={stylexStyles2.rule26}>
          {controller.showEmptyState ? (
            <NoteListEmptyState
              isEmptyDirectory={isEmptyDirectory}
              onCreateNote={controller.handleCreateNote}
            />
          ) : (
            <NoteListEntries controller={controller} />
          )}
        </div>
      </div>
      <NoteListDialogs controller={controller} />
    </>
  )
}

/** NoteList lists notebook directories and note files for the selected source. */
function NoteList(props: NoteListProps) {
  const controller = useNoteListController(props)
  return <NoteListView controller={controller} />
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
