import { useCallback, useMemo, useState } from 'react'
import Markdown from 'markdown-to-jsx'
import {
  LuBookOpen,
  LuFile,
  LuPenLine,
  LuPlus,
  LuSearch,
  LuX,
} from 'react-icons/lu'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { PreBlock } from '@s4wave/app/docs/CodeBlock.js'
import '@s4wave/app/docs/docs-prose.css'
import { Documentation } from '@s4wave/sdk/docs/docs.pb.js'
import { MknodType } from '@s4wave/sdk/unixfs/index.js'
import { keyToIRI, iriToKey } from '@s4wave/sdk/world/graph-utils.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useForgeBlockData } from '@s4wave/web/forge/useForgeBlockData.js'
import {
  useUnixFSRootHandle,
  useUnixFSHandle,
  useUnixFSHandleEntries,
  useUnixFSHandleTextContent,
} from '@s4wave/web/hooks/useUnixFSHandle.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/index.js'
import { cn } from '@s4wave/web/style/utils.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import { MarkdownLink } from './MarkdownLink.js'

export const DocumentationTypeID = 'spacewave-docs/documentation'

// DOC_SOURCE_PREDICATE is the graph predicate linking documentation to its UnixFS source.
const DOC_SOURCE_PREDICATE = '<doc/source>'

// markdownOverrides configures markdown-to-jsx for code blocks and internal links.
const markdownOverrides = {
  overrides: {
    a: { component: MarkdownLink },
    pre: { component: PreBlock },
  },
}

// DocumentationViewer displays a Documentation world object with a file sidebar
// and markdown content viewer.
export function DocumentationViewer({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const ns = useStateNamespace(['docs', objectKey])
  const doc = useForgeBlockData(objectState, Documentation)

  // Resolve the doc/source graph edge to find the linked UnixFS object key.
  const linkedSource = useResource(
    worldState,
    async (world: IWorldState, signal: AbortSignal) => {
      if (!world) return null
      const iri = keyToIRI(objectKey)
      const result = await world.lookupGraphQuads(
        iri,
        DOC_SOURCE_PREDICATE,
        undefined,
        undefined,
        1,
        signal,
      )
      const quads = result.quads ?? []
      if (quads.length === 0 || !quads[0].obj) return null
      return iriToKey(quads[0].obj)
    },
    [objectKey],
  )

  const sourceKey = useMemo(
    () => linkedSource.value ?? '',
    [linkedSource.value],
  )

  // Access the UnixFS root and list directory entries.
  const rootHandle = useUnixFSRootHandle(worldState, sourceKey)
  const entries = useUnixFSHandleEntries(rootHandle, {
    enabled: !!sourceKey,
  })

  // Filter to .md files only.
  const mdEntries = useMemo(() => {
    if (!entries.value) return []
    return entries.value.filter(
      (entry) => !entry.isDir && entry.name.endsWith('.md'),
    )
  }, [entries.value])

  // Persisted state for selected page and editing mode.
  const [selectedPage, setSelectedPage] = useStateAtom<string>(
    ns,
    'selectedPage',
    '',
  )
  const [editing, setEditing] = useStateAtom<boolean>(ns, 'editing', false)
  const [searchQuery, setSearchQuery] = useState('')

  // Filter entries by search query.
  const filteredEntries = useMemo(() => {
    if (!searchQuery) return mdEntries
    const lower = searchQuery.toLowerCase()
    return mdEntries.filter((entry) => entry.name.toLowerCase().includes(lower))
  }, [mdEntries, searchQuery])

  // File handle and content for the selected page.
  const fileHandle = useUnixFSHandle(rootHandle, selectedPage)
  const textResource = useUnixFSHandleTextContent(fileHandle)

  // Edit state for the textarea content.
  const [editContent, setEditContent] = useState<string | null>(null)

  const handleSelectPage = useCallback(
    (name: string) => {
      setSelectedPage(name)
      setEditing(false)
      setEditContent(null)
    },
    [setSelectedPage, setEditing],
  )

  const handleToggleEdit = useCallback(() => {
    if (editing) {
      // Save on exit from edit mode.
      if (editContent !== null) {
        const handle = fileHandle.value
        if (handle) {
          const encoded = new TextEncoder().encode(editContent)
          handle
            .writeAt(0n, encoded)
            .then(() => handle.truncate(BigInt(encoded.byteLength)))
        }
        setEditContent(null)
      }
    } else {
      setEditContent(textResource.value ?? '')
    }
    setEditing((prev) => !prev)
  }, [editing, editContent, fileHandle.value, textResource.value, setEditing])

  const handleCancelEdit = useCallback(() => {
    setEditContent(null)
    setEditing(false)
  }, [setEditing])

  const handleCreatePage = useCallback(async () => {
    const handle = rootHandle.value
    if (!handle) return

    const existing = new Set(mdEntries.map((e) => e.name))
    let name = 'untitled.md'
    let counter = 1
    while (existing.has(name)) {
      name = `untitled-${counter}.md`
      counter++
    }

    await handle.mknod([name], MknodType.FILE)
    const child = await handle.lookup(name)
    const title = name.replace(/\.md$/, '')
    const template = `# ${title}\n`
    const encoded = new TextEncoder().encode(template)
    await child.writeAt(0n, encoded)
    child.release()
    handleSelectPage(name)
  }, [rootHandle.value, mdEntries, handleSelectPage])

  const title = doc?.name || 'Documentation'

  // Gate the viewer on the doc source resource: while the linked UnixFS source
  // is resolving (or the initial directory listing has not arrived yet), show a
  // single LoadingCard in the content chrome instead of flashing partial UI.
  const docLoading =
    linkedSource.loading || (!!sourceKey && entries.loading && !entries.value)
  if (docLoading) {
    return (
      <div className="bg-background-primary flex h-full w-full flex-col">
        <div className="border-foreground/8 flex h-9 shrink-0 items-center border-b px-4">
          <div className="text-foreground flex items-center gap-2 text-sm font-semibold select-none">
            <LuBookOpen className="h-4 w-4" />
            <span className="tracking-tight">{title}</span>
          </div>
        </div>
        <div className="flex flex-1 items-center justify-center p-6">
          <div className="w-full max-w-sm">
            <LoadingCard
              view={{
                state: 'active',
                title: 'Loading documentation',
                detail: 'Resolving the source and reading pages.',
              }}
            />
          </div>
        </div>
      </div>
    )
  }

  // No source linked state.
  if (!sourceKey) {
    return (
      <div className="bg-background-primary flex h-full w-full flex-col">
        <div className="border-foreground/8 flex h-9 shrink-0 items-center border-b px-4">
          <div className="text-foreground flex items-center gap-2 text-sm font-semibold select-none">
            <LuBookOpen className="h-4 w-4" />
            <span className="tracking-tight">{title}</span>
          </div>
        </div>
        <div className="text-muted-foreground flex flex-1 items-center justify-center text-xs">
          No documentation source linked
        </div>
      </div>
    )
  }

  return (
    <div className="bg-background-primary flex h-full w-full overflow-hidden">
      {/* Sidebar */}
      <div
        className="border-border flex flex-col border-r"
        style={{ width: 220, minWidth: 220 }}
      >
        {/* Header */}
        <div className="border-foreground/8 flex h-9 shrink-0 items-center gap-2 border-b px-3">
          <LuBookOpen className="text-foreground h-4 w-4 shrink-0" />
          <span className="text-foreground truncate text-sm font-semibold tracking-tight">
            {title}
          </span>
        </div>

        {/* Search and create */}
        <div className="border-border flex items-center gap-1 border-b px-2 py-1.5">
          <div className="bg-muted flex flex-1 items-center gap-1.5 rounded px-2 py-1">
            <LuSearch className="text-muted-foreground h-3 w-3 shrink-0" />
            <input
              type="text"
              placeholder="Search pages..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="text-foreground placeholder:text-muted-foreground w-full border-none bg-transparent text-xs outline-none"
            />
          </div>
          <button
            type="button"
            className="text-foreground-alt hover:bg-list-hover-background hover:text-foreground flex items-center justify-center rounded p-1.5"
            onClick={handleCreatePage}
            title="New page"
          >
            <LuPlus className="h-3.5 w-3.5" />
          </button>
        </div>

        {/* File listing */}
        <div className="flex-1 overflow-y-auto">
          {filteredEntries.length === 0 ?
            <div className="text-muted-foreground flex flex-col items-center justify-center gap-3 p-6 text-center">
              {mdEntries.length === 0 ?
                <>
                  <span className="text-xs">No pages yet</span>
                  <button
                    type="button"
                    className="bg-brand text-brand-foreground rounded-md px-3 py-1.5 text-xs font-medium hover:opacity-90"
                    onClick={handleCreatePage}
                  >
                    Create first page
                  </button>
                </>
              : <span className="text-xs">No matching pages</span>}
            </div>
          : filteredEntries.map((entry) => {
              const label = entry.name.replace(/\.md$/, '')
              const selected = selectedPage === entry.name
              return (
                <button
                  key={entry.name}
                  type="button"
                  className={cn(
                    'flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs',
                    'hover:bg-list-hover-background',
                    selected &&
                      'bg-list-active-selection-background text-list-active-selection-foreground',
                  )}
                  onClick={() => handleSelectPage(entry.name)}
                >
                  <LuFile className="h-3 w-3 shrink-0" />
                  <span className="truncate">{label}</span>
                </button>
              )
            })
          }
        </div>
      </div>

      {/* Content area */}
      <div className="flex min-w-0 flex-1 flex-col">
        {selectedPage ?
          <>
            {/* Content header */}
            <div className="border-border flex items-center justify-between border-b px-3 py-1.5">
              <span className="text-xs font-medium">
                {selectedPage.replace(/\.md$/, '')}
              </span>
              <div className="flex items-center gap-1">
                {editing && (
                  <button
                    type="button"
                    className="text-foreground-alt hover:bg-list-hover-background flex items-center gap-1 rounded px-2 py-0.5 text-xs"
                    onClick={handleCancelEdit}
                    title="Cancel editing"
                  >
                    <LuX className="h-3 w-3" />
                    Cancel
                  </button>
                )}
                <button
                  type="button"
                  className={cn(
                    'flex items-center gap-1 rounded px-2 py-0.5 text-xs',
                    'hover:bg-list-hover-background',
                    editing ? 'text-brand' : 'text-foreground-alt',
                  )}
                  onClick={handleToggleEdit}
                  title={editing ? 'Save and preview' : 'Edit page'}
                >
                  <LuPenLine className="h-3 w-3" />
                  {editing ? 'Save' : 'Edit'}
                </button>
              </div>
            </div>

            {/* Content body */}
            {textResource.loading ?
              <div className="flex flex-1 items-center justify-center p-4">
                <LoadingInline label="Loading page" tone="muted" size="sm" />
              </div>
            : textResource.error ?
              <div className="text-destructive flex flex-1 flex-col items-center justify-center gap-2 p-4 text-xs">
                <span>Failed to load page</span>
                <span className="text-foreground-alt/50 text-xs">
                  {textResource.error.message}
                </span>
              </div>
            : editing ?
              <div className="flex-1 overflow-auto">
                <textarea
                  className="bg-background-primary text-editor-foreground h-full w-full resize-none border-none p-4 font-mono text-xs outline-none"
                  value={editContent ?? textResource.value ?? ''}
                  onChange={(e) => setEditContent(e.target.value)}
                />
              </div>
            : <div className="flex-1 overflow-auto p-4">
                <div className="docs-prose">
                  <Markdown options={markdownOverrides}>
                    {textResource.value ?? ''}
                  </Markdown>
                </div>
              </div>
            }
          </>
        : <div className="text-muted-foreground flex flex-1 items-center justify-center text-xs">
            Select a page to view
          </div>
        }
      </div>
    </div>
  )
}
