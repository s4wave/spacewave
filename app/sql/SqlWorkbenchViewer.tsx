import { useCallback, useMemo, useState } from 'react'
import {
  LuFileText,
  LuLayoutGrid,
  LuPinOff,
  LuTableProperties,
  LuX,
} from '@s4wave/web/images/icons.js'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import { ObjectViewer } from '@s4wave/web/object/ObjectViewer.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'
import type { ObjectInfo } from '@s4wave/web/object/object.pb.js'

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

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import { groupMarker } from '../../web/style/relations.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    backgroundColor: themeColors['--color-background-primary'],
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule4: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule5: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule6: { padding: `calc(${themeLayout['--spacing']} * 4)` },
  rule7: { display: 'flex', minHeight: 0, flex: 1 },
  rule8: { display: 'flex', minWidth: 0, flex: 1, flexDirection: 'column' },
  rule9: { minHeight: 0, flex: 1 },
  rule10: {
    display: 'flex',
    height: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule11: {
    width: 'var(--sql-sidebar-width)',
    display: 'flex',
    flexShrink: 0,
    flexDirection: 'column',
    overflow: 'auto',
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule12: { height: `calc(${themeLayout['--spacing']} * 64)` },
  rule13: {
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule14: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule15: {
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: '.65rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    textTransform: 'uppercase',
  },
  rule16: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    textAlign: 'left',
  },
  rule17: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule18: {
    flexShrink: 0,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    opacity: {
      default: 0,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]: 1,
      },
    },
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule19: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * .5)`,
    overflowX: 'auto',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: themeLayout['--spacing'],
  },
  rule20: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderTopLeftRadius: '.25rem',
    borderTopRightRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule21: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule22: {
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule23: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule24: {
    maxWidth: `calc(${themeLayout['--spacing']} * 40)`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule25: { color: themeColors['--color-foreground'] },
  rule26: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule27: {
    flexShrink: 0,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
  },
  rule28: { height: '100%', width: '100%' },
})

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: themeLayout['--spacing'],
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

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
  useObjectName(workbench?.displayName)
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
    <div sx={stylexStyles4.rule1}>
      <div sx={stylexStyles4.rule2}>
        <LuLayoutGrid sx={stylexStyles.rule1} aria-hidden />
        <span sx={stylexStyles4.rule3}>SQL Workbench</span>
        {targetDbKey ? (
          <span sx={stylexStyles4.rule4}>{targetDbKey}</span>
        ) : null}
      </div>

      {mutationError ? (
        <div sx={stylexStyles4.rule5}>
          <ErrorState variant="inline" message={mutationError} />
        </div>
      ) : null}

      {workbenchResource.loading && workbench == null ? (
        <div sx={stylexStyles4.rule6}>
          <LoadingInline label="Loading workbench" tone="muted" />
        </div>
      ) : null}
      {workbenchResource.error ? (
        <div sx={stylexStyles4.rule6}>
          <ErrorState
            title="SQL workbench unavailable"
            message={String(workbenchResource.error)}
            onRetry={workbenchResource.retry}
          />
        </div>
      ) : null}

      {workbench ? (
        <div sx={stylexStyles4.rule7}>
          <WorkbenchSidebar
            width={sidebarWidth}
            targetDbKey={targetDbKey}
            pins={pins}
            worldState={worldState}
            onOpenPinned={openPinned}
            onUnpin={unpin}
          />

          <div sx={stylexStyles4.rule8}>
            <TabStrip
              tabs={tabs}
              activeTabId={activeTabId}
              onSelect={selectTab}
              onClose={closeTab}
            />
            <div sx={stylexStyles4.rule9}>
              {activeTab?.objectKey ? (
                <EmbeddedObject
                  key={activeTab.tabId}
                  objectKey={activeTab.objectKey}
                  worldState={worldState}
                  targetDbObjectKey={targetDbKey}
                />
              ) : (
                <div sx={stylexStyles4.rule10}>
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

interface WorkbenchSidebarProps {
  width: number
  targetDbKey: string
  pins: string[]
  worldState: Resource<IWorldState>
  onOpenPinned: (queryKey: string) => void
  onUnpin: (queryKey: string) => void
}

/** WorkbenchSidebar shows the target database schema and the pinned queries. */
function WorkbenchSidebar({
  width,
  targetDbKey,
  pins,
  worldState,
  onOpenPinned,
  onUnpin,
}: WorkbenchSidebarProps) {
  return (
    <div sx={stylexStyles4.rule11} style={{ '--sql-sidebar-width': width }}>
      <SidebarSection title="Database">
        {targetDbKey ? (
          <div sx={stylexStyles4.rule12}>
            <EmbeddedObject
              objectKey={targetDbKey}
              worldState={worldState}
              targetDbObjectKey={targetDbKey}
            />
          </div>
        ) : (
          <div sx={stylexStyles4.rule13}>No target database.</div>
        )}
      </SidebarSection>
      <SidebarSection title="Pinned Queries">
        {pins.length === 0 ? (
          <div sx={stylexStyles4.rule13}>No pinned queries.</div>
        ) : (
          pins.map((queryKey) => (
            <PinnedRow
              key={queryKey}
              queryKey={queryKey}
              onOpen={onOpenPinned}
              onUnpin={onUnpin}
            />
          ))
        )}
      </SidebarSection>
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
    <section sx={stylexStyles4.rule14}>
      <div sx={stylexStyles4.rule15}>{title}</div>
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
    <div sx={[stylexStyles3.rule1, groupMarker]}>
      <button
        type="button"
        onClick={() => onOpen(queryKey)}
        sx={stylexStyles4.rule16}
      >
        <LuFileText sx={stylexStyles.rule2} />
        <span sx={stylexStyles4.rule17}>
          {queryKey.split('/').pop() || queryKey}
        </span>
      </button>
      <button
        type="button"
        aria-label="Unpin query"
        onClick={() => onUnpin(queryKey)}
        sx={stylexStyles4.rule18}
      >
        <LuPinOff sx={stylexStyles.rule3} />
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
    <div sx={stylexStyles4.rule19}>
      {tabs.map((tab) => {
        const active = tab.tabId === activeTabId
        const Icon =
          tab.kind === WorkbenchTabKind.QUERY_RESULT
            ? LuTableProperties
            : LuFileText
        return (
          <div
            key={tab.tabId}
            sx={[
              stylexStyles4.rule20,
              active ? stylexStyles4.rule21 : stylexStyles4.rule22,
            ]}
          >
            <button
              type="button"
              onClick={() => onSelect(tab.tabId ?? '')}
              sx={stylexStyles4.rule23}
            >
              <Icon sx={stylexStyles2.rule1} />
              <span
                sx={[
                  stylexStyles4.rule24,
                  active ? stylexStyles4.rule25 : stylexStyles4.rule26,
                ]}
              >
                {tab.title || tab.objectKey || tab.tabId}
              </span>
            </button>
            <button
              type="button"
              aria-label="Close tab"
              onClick={() => onClose(tab.tabId ?? '')}
              sx={stylexStyles4.rule27}
            >
              <LuX sx={stylexStyles.rule4} />
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
      <div sx={stylexStyles4.rule28}>
        <ObjectViewer
          objectInfo={info}
          worldState={worldState}
          stateNamespace={['sql-workbench', objectKey]}
        />
      </div>
    </SqlWorkbenchTargetDbContext.Provider>
  )
}
