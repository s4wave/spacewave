import { useCallback, useEffect, useMemo, useRef } from 'react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
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
import NoteContentView from './NoteContentView.js'

// DocsViewer is the viewer for spacewave-notes Documentation objects.
// Renders a tree sidebar, content area, and table of contents panel.
function DocsViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const ns = useStateNamespace(['docs'])

  const { state, sources } = useWorldObjectMessageState(
    worldState,
    objectKey,
    Documentation.fromBinary,
  )

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

  // Load root entries to detect index.md for auto-selection.
  const rootHandle = useUnixFSRootHandle(worldState, sourceObjectKey)
  const rootPathHandle = useUnixFSHandle(rootHandle, sourceSubpath)
  const rootEntries = useUnixFSHandleEntries(rootPathHandle)

  // Auto-select index.md when no page is selected.
  const autoSelectedRef = useRef(false)
  useEffect(() => {
    if (selectedPage || autoSelectedRef.current) return
    if (!rootEntries.value) return
    const hasIndex = rootEntries.value.some(
      (e) => !e.isDir && e.name === 'index.md',
    )
    if (hasIndex) {
      autoSelectedRef.current = true
      setSelectedPage('index.md')
    }
  }, [selectedPage, rootEntries.value, setSelectedPage])

  // Read the selected file's text content for the TOC panel.
  const filePath = useMemo(() => {
    if (!selectedPage) return ''
    return sourceSubpath
      ? `${sourceSubpath}/${selectedPage}`
      : selectedPage
  }, [sourceSubpath, selectedPage])

  const tocFileHandle = useUnixFSHandle(rootHandle, filePath)
  const tocTextResource = useUnixFSHandleTextContent(tocFileHandle)
  const tocMarkdown = tocTextResource.value ?? ''

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
    <div className="bg-background-primary flex h-full w-full overflow-hidden">
      {/* Tree sidebar */}
      <div
        className="border-r border-border"
        style={{ width: 220, minWidth: 220 }}
      >
        <DocsSidebar
          source={firstSource}
          worldState={worldState}
          selectedPage={selectedPage}
          onSelectPage={handleSelectPage}
          namespace={ns}
        />
      </div>

      {/* Content area */}
      <div className="min-w-0 flex-1">
        {firstSource?.ref && selectedPage ?
          <NoteContentView
            worldState={worldState}
            sourceRef={firstSource.ref}
            noteName={selectedPage}
            editing={editing}
            onToggleEdit={handleToggleEdit}
          />
        : <div className="text-muted-foreground flex h-full items-center justify-center text-xs">
            Select a page to view
          </div>
        }
      </div>

      {/* TOC panel (shown when viewing a page with headings, not editing) */}
      {selectedPage && !editing && tocMarkdown && (
        <div
          className="border-l border-border"
          style={{ width: 180, minWidth: 180 }}
        >
          <DocsToc markdown={tocMarkdown} />
        </div>
      )}
    </div>
    </ViewerStatusShell>
  )
}

export { DocsViewer, DocsTypeID }
export default DocsViewer
