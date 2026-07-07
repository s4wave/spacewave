import { useCallback, useMemo, useState } from 'react'
import {
  LuFileText,
  LuLayoutGrid,
  LuPinOff,
  LuTableProperties,
  LuX,
} from 'react-icons/lu'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import { ObjectViewer } from '@s4wave/web/object/ObjectViewer.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import type { ObjectInfo } from '@s4wave/web/object/object.pb.js'
import { cn } from '@s4wave/web/style/utils.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import {
  SqlWorkbench,
  SqlWorkbenchTypeID,
} from '@s4wave/sdk/sql/workbench/workbench.js'
import {
  WorkbenchTabKind,
  type WorkbenchTab,
} from '@s4wave/sdk/sql/workbench/workbench.pb.js'

import { SqlWorkbenchTargetDbContext } from './sql-workbench-context.js'

export { SqlWorkbenchTypeID }

const DEFAULT_SIDEBAR_WIDTH = 280

// SqlWorkbenchViewer is the full sql/workbench viewer: a composition frame with
// a database schema sidebar, pinned-query list, and a persisted tab strip whose
// active tab embeds the corresponding sql object viewer. Pin and tab state
// persist through the workbench handle.
export function SqlWorkbenchViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    SqlWorkbench,
    SqlWorkbenchTypeID,
  )

  const workbenchResource = useResource(
    handle,
    async (workbench, signal) => {
      if (!workbench) return null
      return workbench.getWorkbench(signal)
    },
    [],
  )

  const [mutationError, setMutationError] = useState<string | null>(null)

  const workbench = workbenchResource.value?.workbench
  const targetDbKey = workbench?.targetDbObjectKey ?? ''
  const pins = useMemo(
    () => workbench?.pinnedQueryObjectKeys ?? [],
    [workbench],
  )
  const tabs = useMemo(() => workbench?.openTabs ?? [], [workbench])
  const layout = workbench?.layout
  const sidebarWidth = layout?.sidebarWidth || DEFAULT_SIDEBAR_WIDTH

  const [localActiveTabId, setLocalActiveTabId] = useState<string | null>(null)
  const activeTabId =
    localActiveTabId ?? layout?.activeTabId ?? tabs[0]?.tabId ?? null
  const activeTab = tabs.find((tab) => tab.tabId === activeTabId) ?? null

  const runMutation = useCallback(
    async (mutate: (wb: SqlWorkbench) => Promise<void>) => {
      const wb = handle.value
      if (!wb) return
      setMutationError(null)
      try {
        await mutate(wb)
      } catch (err) {
        setMutationError(err instanceof Error ? err.message : String(err))
      }
      workbenchResource.retry()
    },
    [handle.value, workbenchResource],
  )

  // openTab adds or focuses a tab for a query/result object and persists the
  // tab list plus the active tab.
  const openTab = useCallback(
    (tab: WorkbenchTab) => {
      setLocalActiveTabId(tab.tabId ?? null)
      const nextTabs = tabs.some((existing) => existing.tabId === tab.tabId)
        ? tabs
        : [...tabs, tab]
      void runMutation((wb) =>
        wb.setLayout(nextTabs, { ...layout, activeTabId: tab.tabId }),
      )
    },
    [layout, runMutation, tabs],
  )

  const closeTab = useCallback(
    (tabId: string) => {
      const nextTabs = tabs.filter((tab) => tab.tabId !== tabId)
      const nextActive =
        activeTabId === tabId ? (nextTabs[0]?.tabId ?? '') : (activeTabId ?? '')
      setLocalActiveTabId(nextActive || null)
      void runMutation((wb) =>
        wb.setLayout(nextTabs, { ...layout, activeTabId: nextActive }),
      )
    },
    [activeTabId, layout, runMutation, tabs],
  )

  const selectTab = useCallback(
    (tabId: string) => {
      setLocalActiveTabId(tabId)
      void runMutation((wb) =>
        wb.setLayout(tabs, { ...layout, activeTabId: tabId }),
      )
    },
    [layout, runMutation, tabs],
  )

  const unpin = useCallback(
    (queryKey: string) => {
      void runMutation((wb) => wb.removePin(queryKey))
    },
    [runMutation],
  )

  const openPinned = useCallback(
    (queryKey: string) => {
      openTab({
        tabId: `query:${queryKey}`,
        objectKey: queryKey,
        kind: WorkbenchTabKind.QUERY,
        title: queryKey.split('/').pop() || queryKey,
      })
    },
    [openTab],
  )

  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center gap-2 border-b px-4">
        <LuLayoutGrid className="text-foreground-alt size-3.5" aria-hidden />
        <span className="text-foreground text-sm font-semibold tracking-tight select-none">
          SQL Workbench
        </span>
        {targetDbKey ? (
          <span className="text-foreground-alt/50 truncate font-mono text-xs">
            {targetDbKey}
          </span>
        ) : null}
      </div>

      {mutationError ? (
        <div className="px-4 pt-2">
          <ErrorState variant="inline" message={mutationError} />
        </div>
      ) : null}

      {workbenchResource.loading && workbench == null ? (
        <div className="p-4">
          <LoadingInline label="Loading workbench" tone="muted" />
        </div>
      ) : null}
      {workbenchResource.error ? (
        <div className="p-4">
          <ErrorState
            title="SQL workbench unavailable"
            message={String(workbenchResource.error)}
            onRetry={workbenchResource.retry}
          />
        </div>
      ) : null}

      {workbench ? (
        <div className="flex min-h-0 flex-1">
          <div
            className="border-foreground/8 flex shrink-0 flex-col overflow-auto border-r"
            style={{ width: sidebarWidth }}
          >
            <SidebarSection title="Database">
              {targetDbKey ? (
                <div className="h-64">
                  <EmbeddedObject
                    objectKey={targetDbKey}
                    worldState={worldState}
                    targetDbObjectKey={targetDbKey}
                  />
                </div>
              ) : (
                <div className="text-foreground-alt/40 px-3 py-2 text-xs">
                  No target database.
                </div>
              )}
            </SidebarSection>
            <SidebarSection title="Pinned Queries">
              {pins.length === 0 ? (
                <div className="text-foreground-alt/40 px-3 py-2 text-xs">
                  No pinned queries.
                </div>
              ) : (
                pins.map((queryKey) => (
                  <PinnedRow
                    key={queryKey}
                    queryKey={queryKey}
                    onOpen={openPinned}
                    onUnpin={unpin}
                  />
                ))
              )}
            </SidebarSection>
          </div>

          <div className="flex min-w-0 flex-1 flex-col">
            <TabStrip
              tabs={tabs}
              activeTabId={activeTabId}
              onSelect={selectTab}
              onClose={closeTab}
            />
            <div className="min-h-0 flex-1">
              {activeTab?.objectKey ? (
                <EmbeddedObject
                  key={activeTab.tabId}
                  objectKey={activeTab.objectKey}
                  worldState={worldState}
                  targetDbObjectKey={targetDbKey}
                />
              ) : (
                <div className="text-foreground-alt/40 flex h-full items-center justify-center text-xs">
                  Open a pinned query to start.
                </div>
              )}
            </div>
          </div>
        </div>
      ) : null}
    </div>
  )
}

function SidebarSection({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="border-foreground/8 border-b">
      <div className="text-foreground-alt/70 px-3 py-1.5 text-[0.65rem] font-medium tracking-wide uppercase">
        {title}
      </div>
      {children}
    </section>
  )
}

interface PinnedRowProps {
  queryKey: string
  onOpen: (queryKey: string) => void
  onUnpin: (queryKey: string) => void
}

function PinnedRow({ queryKey, onOpen, onUnpin }: PinnedRowProps) {
  return (
    <div className="hover:bg-foreground/5 group flex items-center gap-1.5 px-3 py-1">
      <button
        type="button"
        onClick={() => onOpen(queryKey)}
        className="flex min-w-0 flex-1 items-center gap-1.5 text-left"
      >
        <LuFileText className="text-foreground-alt/60 size-3.5 shrink-0" />
        <span className="text-foreground min-w-0 flex-1 truncate font-mono text-xs">
          {queryKey.split('/').pop() || queryKey}
        </span>
      </button>
      <button
        type="button"
        aria-label="Unpin query"
        onClick={() => onUnpin(queryKey)}
        className="text-foreground-alt/40 hover:text-foreground shrink-0 opacity-0 transition-opacity group-hover:opacity-100"
      >
        <LuPinOff className="size-3.5" />
      </button>
    </div>
  )
}

interface TabStripProps {
  tabs: WorkbenchTab[]
  activeTabId: string | null
  onSelect: (tabId: string) => void
  onClose: (tabId: string) => void
}

function TabStrip({ tabs, activeTabId, onSelect, onClose }: TabStripProps) {
  if (tabs.length === 0) return null
  return (
    <div className="border-foreground/8 flex shrink-0 items-center gap-0.5 overflow-x-auto border-b px-1">
      {tabs.map((tab) => {
        const active = tab.tabId === activeTabId
        const Icon =
          tab.kind === WorkbenchTabKind.QUERY_RESULT
            ? LuTableProperties
            : LuFileText
        return (
          <div
            key={tab.tabId}
            className={cn(
              'flex items-center gap-1.5 rounded-t px-2 py-1.5',
              active ? 'bg-foreground/5' : 'hover:bg-foreground/5',
            )}
          >
            <button
              type="button"
              onClick={() => onSelect(tab.tabId ?? '')}
              className="flex items-center gap-1.5"
            >
              <Icon className="text-foreground-alt/60 size-3 shrink-0" />
              <span
                className={cn(
                  'max-w-40 truncate text-xs',
                  active ? 'text-foreground' : 'text-foreground-alt/70',
                )}
              >
                {tab.title || tab.objectKey || tab.tabId}
              </span>
            </button>
            <button
              type="button"
              aria-label="Close tab"
              onClick={() => onClose(tab.tabId ?? '')}
              className="text-foreground-alt/40 hover:text-foreground shrink-0"
            >
              <LuX className="size-3" />
            </button>
          </div>
        )
      })}
    </div>
  )
}

interface EmbeddedObjectProps {
  objectKey: string
  worldState: Resource<IWorldState>
  targetDbObjectKey: string
}

// EmbeddedObject renders a world object's registered viewer inside the
// workbench by constructing its ObjectInfo from the key. The object type is
// resolved by the viewer from the world object handle.
function EmbeddedObject({
  objectKey,
  worldState,
  targetDbObjectKey,
}: EmbeddedObjectProps) {
  const info: ObjectInfo = useMemo(
    () => ({ info: { case: 'worldObjectInfo', value: { objectKey } } }),
    [objectKey],
  )
  return (
    <SqlWorkbenchTargetDbContext.Provider value={targetDbObjectKey}>
      <div className="h-full w-full">
        <ObjectViewer
          objectInfo={info}
          worldState={worldState}
          stateNamespace={['sql-workbench', objectKey]}
        />
      </div>
    </SqlWorkbenchTargetDbContext.Provider>
  )
}
