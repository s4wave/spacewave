import { useCallback, useMemo, useState } from 'react'
import { LuDatabase, LuPlus } from 'react-icons/lu'

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
    <div className="bg-background-primary flex h-full w-full flex-col">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center justify-between border-b px-4">
        <div className="text-foreground flex items-center gap-2 text-sm font-semibold tracking-tight select-none">
          <LuDatabase className="text-foreground-alt size-3.5" aria-hidden />
          Key/Value Store
          <span className="text-foreground-alt/50 text-xs">
            {rows.length} {rows.length === 1 ? 'key' : 'keys'}
          </span>
        </div>
        <DashboardButton
          icon={<LuPlus className="size-3.5" />}
          onClick={handleStartCreate}
        >
          New Key
        </DashboardButton>
      </div>

      {keysResource.error ? (
        <div className="p-4">
          <ErrorState
            title="KV store unavailable"
            message={String(keysResource.error)}
            onRetry={keysResource.retry}
          />
        </div>
      ) : (
        <div className="flex min-h-0 flex-1">
          <div className="border-foreground/8 flex w-1/3 min-w-48 flex-col border-r">
            <KvKeyListToolbar
              prefix={prefix}
              onPrefixChange={setPrefix}
              sortDirection={sortDirection}
              onToggleSortDirection={toggleSortDirection}
            />
            {keysResource.loading && rows.length === 0 ? (
              <div className="p-3">
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

          <div className="min-w-0 flex-1">
            {creating ? (
              <div className="overflow-auto p-3">
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
              <div className="text-foreground-alt/40 flex h-full items-center justify-center text-xs">
                Select a key to view its value.
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
