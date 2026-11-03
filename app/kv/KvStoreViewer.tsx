import { useCallback, useMemo, useState } from 'react'
import { LuDatabase, LuPlus } from '@s4wave/web/images/icons.js'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import { KvStore, KvStoreTypeID } from '@s4wave/sdk/kv/kv.js'

import { KvCreateKeyForm } from './KvCreateKeyForm.js'
import { KvKeyDetail } from './KvKeyDetail.js'
import { KvKeyList } from './KvKeyList.js'
import { KvKeyListToolbar } from './KvKeyListToolbar.js'
import { bytesToText } from './kv-encoding.js'
import type { KvKeyRow, KvSortDirection } from './kv-viewer.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
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
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
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
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule5: { padding: `calc(${themeLayout['--spacing']} * 4)` },
  rule6: { display: 'flex', minHeight: 0, flex: 1 },
  rule7: {
    display: 'flex',
    width: '33.3333%',
    minWidth: `calc(${themeLayout['--spacing']} * 48)`,
    flexDirection: 'column',
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule8: { padding: `calc(${themeLayout['--spacing']} * 3)` },
  rule9: { minWidth: 0, flex: 1 },
  rule10: {
    overflow: 'auto',
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule11: {
    display: 'flex',
    height: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
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
  },
})

export { KvStoreTypeID }

// KvStoreViewer is the full kv/store viewer: a watch-driven key list with byte
// lengths, a prefix filter and sort toggle, a value detail pane with display
// toggles, and create/update/delete flows with optimistic rollback.
export function KvStoreViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    KvStore,
    KvStoreTypeID,
  )

  const keysResource = useResource(
    handle,
    async (kv, signal) => {
      if (!kv) return null
      const entries = await kv.scanKeys(new Uint8Array(), signal)
      return entries.map<KvKeyRow>((entry) => ({
        ...entry,
        label: bytesToText(entry.key),
      }))
    },
    [],
  )

  const [prefix, setPrefix] = useState('')
  const [sortDirection, setSortDirection] = useState<KvSortDirection>('asc')
  const [selectedKey, setSelectedKey] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [mutationError, setMutationError] = useState<string | null>(null)

  const rows = useMemo(() => keysResource.value ?? [], [keysResource.value])

  const visibleRows = useMemo(() => {
    const filtered = prefix
      ? rows.filter((row) => row.label.startsWith(prefix))
      : rows
    const sorted = filtered.toSorted((a, b) =>
      a.label < b.label ? -1 : a.label > b.label ? 1 : 0,
    )
    return sortDirection === 'asc' ? sorted : sorted.toReversed()
  }, [rows, prefix, sortDirection])

  const selectedRow = useMemo(
    () => visibleRows.find((row) => row.label === selectedKey) ?? null,
    [visibleRows, selectedKey],
  )

  const toggleSortDirection = useCallback(() => {
    setSortDirection((current) => (current === 'asc' ? 'desc' : 'asc'))
  }, [])

  const handleSelectKey = useCallback((row: KvKeyRow) => {
    setSelectedKey(row.label)
    setCreating(false)
    setMutationError(null)
  }, [])

  const handleStartCreate = useCallback(() => {
    setCreating(true)
    setSelectedKey(null)
    setMutationError(null)
  }, [])

  // runMutation applies a write then reloads the key list so the watched list
  // reconciles. A failure surfaces inline and returns false; the reload rolls
  // back any optimistic list state that did not actually persist. It never
  // throws, so callers finalize selection only on success.
  const runMutation = useCallback(
    async (mutate: (kv: KvStore) => Promise<void>): Promise<boolean> => {
      const kv = handle.value
      if (!kv) return false
      setMutationError(null)
      let ok = false
      try {
        await mutate(kv)
        ok = true
      } catch (err) {
        setMutationError(err instanceof Error ? err.message : String(err))
      }
      keysResource.retry()
      return ok
    },
    [handle.value, keysResource],
  )

  const handleCreate = useCallback(
    async (key: Uint8Array, value: Uint8Array) => {
      const ok = await runMutation((kv) =>
        kv.withTransaction(true, async (tx) => {
          await tx.set(key, value)
        }),
      )
      if (!ok) return
      setCreating(false)
      setSelectedKey(bytesToText(key))
    },
    [runMutation],
  )

  const handleSave = useCallback(
    async (value: Uint8Array) => {
      const row = selectedRow
      if (!row) return
      await runMutation((kv) =>
        kv.withTransaction(true, async (tx) => {
          await tx.set(row.key, value)
        }),
      )
    },
    [runMutation, selectedRow],
  )

  const handleDelete = useCallback(async () => {
    const row = selectedRow
    if (!row) return
    const ok = await runMutation((kv) =>
      kv.withTransaction(true, async (tx) => {
        await tx.delete(row.key)
      }),
    )
    if (ok) setSelectedKey(null)
  }, [runMutation, selectedRow])

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          <LuDatabase sx={stylexStyles.rule1} aria-hidden />
          Key/Value Store
          <span sx={stylexStyles2.rule4}>
            {rows.length} {rows.length === 1 ? 'key' : 'keys'}
          </span>
        </div>
        <DashboardButton
          icon={<LuPlus sx={stylexStyles.rule2} />}
          onClick={handleStartCreate}
        >
          New Key
        </DashboardButton>
      </div>

      {keysResource.error ? (
        <div sx={stylexStyles2.rule5}>
          <ErrorState
            title="KV store unavailable"
            message={String(keysResource.error)}
            onRetry={keysResource.retry}
          />
        </div>
      ) : (
        <div sx={stylexStyles2.rule6}>
          <div sx={stylexStyles2.rule7}>
            <KvKeyListToolbar
              prefix={prefix}
              onPrefixChange={setPrefix}
              sortDirection={sortDirection}
              onToggleSortDirection={toggleSortDirection}
            />
            {keysResource.loading && rows.length === 0 ? (
              <div sx={stylexStyles2.rule8}>
                <LoadingInline label="Loading keys" tone="muted" />
              </div>
            ) : (
              <KvKeyList
                rows={visibleRows}
                selectedKey={selectedKey}
                onSelectKey={handleSelectKey}
                filtered={prefix.length > 0}
              />
            )}
          </div>

          <div sx={stylexStyles2.rule9}>
            {creating ? (
              <div sx={stylexStyles2.rule10}>
                <KvCreateKeyForm
                  onCreate={handleCreate}
                  onCancel={() => setCreating(false)}
                  mutationError={mutationError}
                />
              </div>
            ) : selectedRow ? (
              <KvKeyDetail
                handle={handle}
                row={selectedRow}
                onSave={handleSave}
                onDelete={handleDelete}
                mutationError={mutationError}
              />
            ) : (
              <div sx={stylexStyles2.rule11}>
                Select a key to view its value.
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
