import { useCallback, useMemo } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { StateNamespace } from '@s4wave/web/state/index.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import {
  useUnixFSRootHandle,
  useUnixFSHandle,
  useUnixFSHandleEntries,
} from '@s4wave/web/hooks/useUnixFSHandle.js'
import { useStateAtom } from '@s4wave/web/state/index.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  LuChevronDown,
  LuChevronRight,
  LuFile,
  LuFolder,
  LuFolderOpen,
} from 'react-icons/lu'

import type { NotebookSource } from '../proto/notebook.pb.js'

interface DocsSidebarProps {
  source: NotebookSource | undefined
  worldState: Resource<IWorldState>
  selectedPage: string
  onSelectPage: (path: string) => void
  namespace: StateNamespace
}

// DocsSidebar renders a tree-structured folder navigation for documentation.
function DocsSidebar({
  source,
  worldState,
  selectedPage,
  onSelectPage,
  namespace,
}: DocsSidebarProps) {
  const parsed = useMemo(() => {
    if (!source?.ref) return null
    return parseObjectUri(source.ref)
  }, [source])

  const objectKey = parsed?.objectKey ?? ''
  const subpath = parsed?.path ?? ''

  const rootHandle = useUnixFSRootHandle(worldState, objectKey)
  const pathHandle = useUnixFSHandle(rootHandle, subpath)
  const entriesResource = useUnixFSHandleEntries(pathHandle)

  if (!source) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center text-xs">
        No source configured
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

  return (
    <div className="flex h-full flex-col overflow-y-auto">
      <div className="text-foreground-alt border-b border-border px-3 py-2 text-xs font-medium uppercase tracking-wide">
        Pages
      </div>
      <div className="flex-1 overflow-y-auto py-1">
        <TreeEntries
          entries={entriesResource.value ?? []}
          worldState={worldState}
          rootHandle={rootHandle}
          basePath={subpath}
          currentPath=""
          selectedPage={selectedPage}
          onSelectPage={onSelectPage}
          namespace={namespace}
          depth={0}
        />
      </div>
    </div>
  )
}

interface TreeEntriesProps {
  entries: FileEntry[]
  worldState: Resource<IWorldState>
  rootHandle: Resource<import('@s4wave/sdk/unixfs/handle.js').FSHandle>
  basePath: string
  currentPath: string
  selectedPage: string
  onSelectPage: (path: string) => void
  namespace: StateNamespace
  depth: number
}

// TreeEntries renders directory entries as a tree with expandable folders.
function TreeEntries({
  entries,
  worldState,
  rootHandle,
  basePath,
  currentPath,
  selectedPage,
  onSelectPage,
  namespace,
  depth,
}: TreeEntriesProps) {
  // Sort: directories first, then files, alphabetical within each group.
  const sorted = useMemo(() => {
    const dirs = entries.filter((e) => e.isDir)
    const files = entries.filter(
      (e) => !e.isDir && e.name.endsWith('.md'),
    )
    dirs.sort((a, b) => a.name.localeCompare(b.name))
    files.sort((a, b) => a.name.localeCompare(b.name))
    return [...dirs, ...files]
  }, [entries])

  return (
    <>
      {sorted.map((entry) => {
        const entryPath = currentPath
          ? `${currentPath}/${entry.name}`
          : entry.name
        if (entry.isDir) {
          return (
            <FolderNode
              key={entry.name}
              name={entry.name}
              worldState={worldState}
              rootHandle={rootHandle}
              basePath={basePath}
              dirPath={entryPath}
              selectedPage={selectedPage}
              onSelectPage={onSelectPage}
              namespace={namespace}
              depth={depth}
            />
          )
        }
        return (
          <FileNode
            key={entry.name}
            name={entry.name}
            path={entryPath}
            selected={selectedPage === entryPath}
            onSelect={onSelectPage}
            depth={depth}
          />
        )
      })}
    </>
  )
}

interface FolderNodeProps {
  name: string
  worldState: Resource<IWorldState>
  rootHandle: Resource<import('@s4wave/sdk/unixfs/handle.js').FSHandle>
  basePath: string
  dirPath: string
  selectedPage: string
  onSelectPage: (path: string) => void
  namespace: StateNamespace
  depth: number
}

// FolderNode renders a collapsible folder in the tree.
function FolderNode({
  name,
  worldState,
  rootHandle,
  basePath,
  dirPath,
  selectedPage,
  onSelectPage,
  namespace,
  depth,
}: FolderNodeProps) {
  const stateKey = `folder_${dirPath}`
  const [expanded, setExpanded] = useStateAtom<boolean>(
    namespace,
    stateKey,
    false,
  )

  const handleToggle = useCallback(() => {
    setExpanded((prev) => !prev)
  }, [setExpanded])

  // Resolve the subdirectory handle for loading children when expanded.
  const fullPath = basePath ? `${basePath}/${dirPath}` : dirPath
  const dirHandle = useUnixFSHandle(rootHandle, fullPath)
  const childEntries = useUnixFSHandleEntries(dirHandle, {
    enabled: expanded,
  })

  const paddingLeft = 8 + depth * 16

  return (
    <div>
      <button
        type="button"
        className={cn(
          'flex w-full items-center gap-1.5 py-1 pr-2 text-left text-xs',
          'hover:bg-list-hover-background',
        )}
        style={{ paddingLeft }}
        onClick={handleToggle}
      >
        {expanded ?
          <LuChevronDown className="h-3 w-3 shrink-0" />
        : <LuChevronRight className="h-3 w-3 shrink-0" />}
        {expanded ?
          <LuFolderOpen className="h-3 w-3 shrink-0" />
        : <LuFolder className="h-3 w-3 shrink-0" />}
        <span className="truncate">{name}</span>
      </button>
      {expanded && childEntries.value && (
        <TreeEntries
          entries={childEntries.value}
          worldState={worldState}
          rootHandle={rootHandle}
          basePath={basePath}
          currentPath={dirPath}
          selectedPage={selectedPage}
          onSelectPage={onSelectPage}
          namespace={namespace}
          depth={depth + 1}
        />
      )}
    </div>
  )
}

interface FileNodeProps {
  name: string
  path: string
  selected: boolean
  onSelect: (path: string) => void
  depth: number
}

// FileNode renders a clickable .md file leaf in the tree.
function FileNode({ name, path, selected, onSelect, depth }: FileNodeProps) {
  const title = name.replace(/\.md$/, '')
  const paddingLeft = 8 + depth * 16

  const handleClick = useCallback(() => {
    onSelect(path)
  }, [onSelect, path])

  return (
    <button
      type="button"
      className={cn(
        'flex w-full items-center gap-1.5 py-1 pr-2 text-left text-xs',
        'hover:bg-list-hover-background',
        selected &&
          'bg-list-active-selection-background text-list-active-selection-foreground',
      )}
      style={{ paddingLeft }}
      onClick={handleClick}
    >
      <LuFile className="h-3 w-3 shrink-0" />
      <span className="truncate">{title}</span>
    </button>
  )
}

export default DocsSidebar
