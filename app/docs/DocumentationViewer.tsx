import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { LuBookOpen } from 'react-icons/lu'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { Documentation } from '@s4wave/sdk/docs/docs.pb.js'
import { useForgeBlockData } from '@s4wave/web/forge/useForgeBlockData.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import {
  useUnixFSHandle,
  useUnixFSHandleEntries,
  useUnixFSHandleTextContent,
  useUnixFSRootHandle,
} from '@s4wave/web/hooks/useUnixFSHandle.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/index.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { keyToIRI, iriToKey } from '@s4wave/sdk/world/graph-utils.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { DocumentationEditor } from './DocumentationEditor.js'
import { DocumentationNavigation } from './DocumentationNavigation.js'
import { createDocumentationPage } from './documentation-operations.js'
import './docs-prose.css'

export const DocumentationTypeID = 'spacewave-docs/documentation'

const DOC_SOURCE_PREDICATE = '<doc/source>'

// DocumentationViewer resolves the documentation source and composes its page
// navigation and selected-page editor.
export function DocumentationViewer({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const namespace = useStateNamespace(['docs', objectKey])
  const doc = useForgeBlockData(objectState, DocumentationTypeID, Documentation)
  const linkedSource = useResource(
    worldState,
    async (world: IWorldState, signal: AbortSignal) => {
      if (!world) return null
      const result = await world.lookupGraphQuads(
        keyToIRI(objectKey),
        DOC_SOURCE_PREDICATE,
        undefined,
        undefined,
        1,
        signal,
      )
      const object = result.quads?.[0]?.obj
      return object ? iriToKey(object) : null
    },
    [objectKey],
  )
  const sourceKey = linkedSource.value ?? ''
  const rootHandle = useUnixFSRootHandle(worldState, sourceKey)
  const entries = useUnixFSHandleEntries(rootHandle, { enabled: !!sourceKey })
  const pages = useMemo(
    () =>
      (entries.value ?? []).flatMap((entry) =>
        !entry.isDir && entry.name?.endsWith('.md')
          ? [{ name: entry.name }]
          : [],
      ),
    [entries.value],
  )
  const [selectedPage, setSelectedPage] = useStateAtom<string>(
    namespace,
    'selectedPage',
    '',
  )
  const [editing, setEditing] = useStateAtom<boolean>(
    namespace,
    'editing',
    false,
  )
  const fileHandle = useUnixFSHandle(rootHandle, selectedPage)
  const text = useUnixFSHandleTextContent(fileHandle)
  const [portalContainer, setPortalContainer] = useState<HTMLDivElement | null>(
    null,
  )
  const createOperation = useRef(0)
  const createAbort = useRef<AbortController | null>(null)

  useEffect(
    () => () => {
      createOperation.current++
      createAbort.current?.abort()
    },
    [],
  )

  const selectPage = useCallback(
    (name: string) => {
      setSelectedPage(name)
      setEditing(false)
    },
    [setEditing, setSelectedPage],
  )
  const createPage = useCallback(async () => {
    if (!rootHandle.value) return
    const current = ++createOperation.current
    const controller = new AbortController()
    createAbort.current?.abort()
    createAbort.current = controller
    try {
      const name = await createDocumentationPage(
        rootHandle.value,
        pages.map((entry) => entry.name),
        controller.signal,
      )
      if (createOperation.current === current && !controller.signal.aborted) {
        selectPage(name)
      }
    } finally {
      if (createOperation.current === current) createAbort.current = null
    }
  }, [pages, rootHandle.value, selectPage])

  const title = doc?.name || 'Documentation'
  const loading =
    linkedSource.loading || (!!sourceKey && entries.loading && !entries.value)

  if (loading) {
    return (
      <div className="bg-background-primary flex h-full w-full flex-col">
        <div className="border-foreground/8 flex h-9 shrink-0 items-center border-b px-4">
          <div className="text-foreground flex items-center gap-2 text-sm font-semibold select-none">
            <LuBookOpen className="size-4" />
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

  if (!sourceKey) {
    return (
      <div className="bg-background-primary flex h-full w-full flex-col">
        <div className="border-foreground/8 flex h-9 shrink-0 items-center border-b px-4">
          <div className="text-foreground flex items-center gap-2 text-sm font-semibold select-none">
            <LuBookOpen className="size-4" />
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
    <div
      ref={setPortalContainer}
      className="bg-background-primary @container relative flex h-full w-full flex-col overflow-hidden"
    >
      <DocumentationNavigation
        title={title}
        entries={pages}
        selectedPage={selectedPage}
        portalContainer={portalContainer}
        onSelectPage={selectPage}
        onCreatePage={createPage}
      >
        <DocumentationEditor
          key={selectedPage}
          page={selectedPage}
          handle={fileHandle}
          text={text}
          editing={editing}
          setEditing={setEditing}
        />
      </DocumentationNavigation>
    </div>
  )
}
