import { useCallback, useEffect, useMemo, useRef } from 'react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'
import { ViewerStatusShell } from '@s4wave/web/object/ViewerStatusShell.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/index.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import {
  useUnixFSRootHandle,
  useUnixFSHandle,
  useUnixFSHandleEntries,
  useUnixFSHandleTextContent,
} from '@s4wave/web/hooks/useUnixFSHandle.js'

import { Documentation } from './proto/docs.pb.js'
import { DocsTypeID } from './sdk/docs.js'
import { useWorldObjectMessageState } from './useWorldObjectMessageState.js'

import DocsSidebar from './docs/DocsSidebar.js'
import DocsToc from './docs/DocsToc.js'
import { getNoteFileFormat } from './note-files.js'
import NoteContentView from './NoteContentView.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '@s4wave/web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-primary'],
    display: 'flex',
    height: '100%',
    width: '100%',
  },
  rule2: {
    borderColor: themeColors['--color-border'],
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    minWidth: `calc(${themeLayout['--spacing']} * 55)`,
    width: `calc(${themeLayout['--spacing']} * 55)`,
  },
  rule3: { flex: 1, minWidth: 0 },
  rule4: {
    alignItems: 'center',
    color: themeColors['--color-muted-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: '100%',
  },
  rule5: {
    borderColor: themeColors['--color-border'],
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: '1px',
    minWidth: `calc(${themeLayout['--spacing']} * 45)`,
    width: `calc(${themeLayout['--spacing']} * 45)`,
  },
})

// DocsViewer is the viewer for Notes Documentation objects.
// Renders a tree sidebar, content area, and table of contents panel.
function DocsViewer({ objectInfo, worldState }: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const ns = useStateNamespace(['docs'])

  const { state, sources } = useWorldObjectMessageState(
    worldState,
    objectKey,
    Documentation.fromBinary,
  )
  useObjectName(state.value?.name)

  const firstSource = sources[0]

  // Persisted state for selected page and editing mode.
  const [selectedPage, setSelectedPage] = useStateAtom<string>(
    ns,
    'selectedPage',
    '',
  )
  const [editing, setEditing] = useStateAtom<boolean>(ns, 'editing', false)

  // Parse source ref for UnixFS access.
  const parsed = useMemo(() => {
    if (!firstSource?.ref) return null
    return parseObjectUri(firstSource.ref)
  }, [firstSource])

  const sourceObjectKey = parsed?.objectKey ?? ''
  const sourceSubpath = parsed?.path ?? ''

  // Load root entries to detect an index page for auto-selection.
  const rootHandle = useUnixFSRootHandle(worldState, sourceObjectKey)
  const rootPathHandle = useUnixFSHandle(rootHandle, sourceSubpath)
  const rootEntries = useUnixFSHandleEntries(rootPathHandle)
  // Auto-select index.md or index.org when no page is selected.
  const autoSelectedRef = useRef(false)
  useEffect(() => {
    if (selectedPage || autoSelectedRef.current) return
    if (!rootEntries.value) return
    const indexEntry = rootEntries.value.find(
      (e) => !e.isDir && (e.name === 'index.md' || e.name === 'index.org'),
    )
    if (indexEntry) {
      autoSelectedRef.current = true
      setSelectedPage(indexEntry.name)
    }
  }, [selectedPage, rootEntries.value, setSelectedPage])

  const selectedPageFormat = getNoteFileFormat(selectedPage) ?? 'markdown'

  // Read the selected file's text content for the TOC panel.
  const filePath = useMemo(() => {
    if (!selectedPage) return ''
    return sourceSubpath ? `${sourceSubpath}/${selectedPage}` : selectedPage
  }, [sourceSubpath, selectedPage])

  const tocFileHandle = useUnixFSHandle(rootHandle, filePath)
  const tocTextResource = useUnixFSHandleTextContent(tocFileHandle)
  const tocContent = tocTextResource.value ?? ''

  const handleSelectPage = useCallback(
    (path: string) => {
      setSelectedPage(path)
      setEditing(false)
    },
    [setSelectedPage, setEditing],
  )

  const handleToggleEdit = useCallback(() => {
    setEditing((prev) => !prev)
  }, [setEditing])

  return (
    <ViewerStatusShell
      resource={state}
      state={state}
      loadingText="Loading documentation..."
      emptyText="No sources configured for this documentation"
      sources={sources}
    >
      <div data-docs-viewer sx={stylexStyles.rule1}>
        {/* Tree sidebar */}
        <div sx={stylexStyles.rule2}>
          <DocsSidebar
            source={firstSource}
            worldState={worldState}
            selectedPage={selectedPage}
            onSelectPage={handleSelectPage}
            namespace={ns}
          />
        </div>

        {/* Content area */}
        <div sx={stylexStyles.rule3}>
          {firstSource?.ref && selectedPage ? (
            <NoteContentView
              worldState={worldState}
              sourceRef={firstSource.ref}
              noteName={selectedPage}
              editing={editing}
              onToggleEdit={handleToggleEdit}
            />
          ) : (
            <div sx={stylexStyles.rule4}>Select a page to view</div>
          )}
        </div>

        {/* TOC panel (shown when viewing a page with headings, not editing) */}
        {selectedPage && !editing && tocContent && (
          <div sx={stylexStyles.rule5}>
            <DocsToc content={tocContent} format={selectedPageFormat} />
          </div>
        )}
      </div>
    </ViewerStatusShell>
  )
}

export { DocsViewer, DocsTypeID }
export default DocsViewer
