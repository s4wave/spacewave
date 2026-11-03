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

import {
  LuChevronDown,
  LuChevronRight,
  LuFile,
  LuFolder,
  LuFolderOpen,
} from '@s4wave/web/images/icons.js'

import type { NotebookSource } from '../proto/notebook.pb.js'
import { isNoteFileName, stripNoteFileExtension } from '../note-files.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '@s4wave/web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  utilityState as themeUtilityState,
} from '@s4wave/web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    alignItems: 'center',
    color: themeColors['--color-muted-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: '100%',
  },
  rule2: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    color: themeColors['--color-muted-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    height: '100%',
  },
  rule3: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    color: themeColors['--color-destructive'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    height: '100%',
  },
  rule4: {
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    overflowY: 'auto',
  },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    borderColor: themeColors['--color-border'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule6: { flex: 1, paddingBlock: themeLayout['--spacing'], overflowY: 'auto' },
  rule7: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: themeLayout['--spacing'],
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'left',
    paddingLeft: 'var(--docs-tree-padding-left)',
    paddingRight: `calc(${themeLayout['--spacing']} * 2)`,
    width: '100%',
  },
  rule8: { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' },
  rule9: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: themeLayout['--spacing'],
    alignItems: 'center',
    backgroundColor: {
      default: themeColors['--color-list-active-selection-background'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    color: themeColors['--color-list-active-selection-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'left',
    paddingLeft: 'var(--docs-tree-padding-left)',
    paddingRight: `calc(${themeLayout['--spacing']} * 2)`,
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

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
    return <div sx={stylexStyles2.rule1}>No source configured</div>
  }

  if (!objectKey) {
    return <div sx={stylexStyles2.rule2}>Invalid source ref</div>
  }

  if (entriesResource.loading) {
    return <div sx={stylexStyles2.rule1}>Loading…</div>
  }

  if (entriesResource.error) {
    return <div sx={stylexStyles2.rule3}>{entriesResource.error.message}</div>
  }

  return (
    <div sx={stylexStyles2.rule4}>
      <div sx={stylexStyles2.rule5}>Pages</div>
      <div sx={stylexStyles2.rule6}>
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
    const files = entries.filter((e) => !e.isDir && isNoteFileName(e.name))
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
        sx={[stylexStyles2.rule7]}
        style={{ '--docs-tree-padding-left': paddingLeft }}
        onClick={handleToggle}
      >
        {expanded ? (
          <LuChevronDown sx={stylexStyles.rule1} />
        ) : (
          <LuChevronRight sx={stylexStyles.rule1} />
        )}
        {expanded ? (
          <LuFolderOpen sx={stylexStyles.rule1} />
        ) : (
          <LuFolder sx={stylexStyles.rule1} />
        )}
        <span sx={stylexStyles2.rule8}>{name}</span>
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

// FileNode renders a clickable note file leaf in the tree.
function FileNode({ name, path, selected, onSelect, depth }: FileNodeProps) {
  const title = stripNoteFileExtension(name)
  const paddingLeft = 8 + depth * 16

  const handleDocSelect = useCallback(() => {
    onSelect(path)
  }, [onSelect, path])

  return (
    <button
      type="button"
      sx={selected ? stylexStyles2.rule9 : stylexStyles2.rule7}
      style={{ '--docs-tree-padding-left': paddingLeft }}
      onClick={handleDocSelect}
    >
      <LuFile sx={stylexStyles.rule1} />
      <span sx={stylexStyles2.rule8}>{title}</span>
    </button>
  )
}

export default DocsSidebar
