import { useCallback, useMemo, useReducer, useState } from 'react'
import { LuDatabase, LuPlus } from 'react-icons/lu'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { ViewerStatus } from '@s4wave/web/object/ViewerStatus.js'
import { ViewerBar } from '@s4wave/web/object/ViewerBar.js'
import {
  type ListStateHandle,
  type ListUpdate,
  useListState,
} from '@s4wave/web/ui/list'

import { KvStore, KvStoreTypeID } from '@s4wave/sdk/kv/kv.js'

import { KvCreateKeyForm } from './KvCreateKeyForm.js'
import { KvKeyDetail } from './KvKeyDetail.js'
import { KvKeyList } from './KvKeyList.js'
import { KvKeyListToolbar } from './KvKeyListToolbar.js'
import { bytesToText } from './kv-encoding.js'
import type { KvKeyRow } from './kv-viewer.js'

export { KvStoreTypeID }

// MutationError is a failed write and the key it was for; null for a create.
interface MutationError {
  key: string | null
  message: string
}

// PaneEvent moves the right pane: create opens the create form; select is a
// change to the list's selection, carrying how many keys it now names; close
// is Cancel.
type PaneEvent =
  | { type: 'create' }
  | { type: 'select'; keys: number }
  | { type: 'close' }

// creatingAfter is whether the right pane shows the create form after event.
// The form opens on an empty selection. Selecting one key closes it, since that
// key's detail takes the pane, so the form never waits behind a selected key
// to return when the selection empties; its draft is dropped then. Selecting
// several keys or none leaves the pane to the form and keeps its draft.
function creatingAfter(creating: boolean, event: PaneEvent): boolean {
  switch (event.type) {
    case 'create':
      return true
    case 'select':
      return creating && event.keys !== 1
    case 'close':
      return false
  }
}

// KvStoreViewer is the full kv/store viewer: a watch-driven table of keys with
// byte lengths, a prefix filter, a value detail pane with display toggles, and
// create/update/delete flows with optimistic rollback.
export function KvStoreViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  // Open the store's typed handle.
  const objectKey = getObjectKey(objectInfo)
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    KvStore,
    KvStoreTypeID,
  )

  // Read every key with its value's size.
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

  // The prefix filter, the create form, and the last failed write.
  const [prefix, setPrefix] = useState('')
  const [creating, paneEvent] = useReducer(creatingAfter, false)
  const [mutationError, setMutationError] = useState<MutationError | null>(null)

  // The key list's selection and sort; the detail shows the one selected key.
  const listState = useListState(null, 'kvKeys', {
    sortKey: 'key',
    sortDirection: 'asc',
  })
  const [list, updateList] = listState
  const selectedKey =
    list.selectedIds?.length === 1 ? list.selectedIds[0] : null

  // updateSelection is the list update every change goes through, the table's
  // and the viewer's own. Selecting one key gives the pane to its detail, which
  // closes the create form and drops the last write error, since the editor it
  // belonged to is gone. The list's update runs its function before it
  // returns.
  const updateSelection = useCallback<ListUpdate>(
    (update) => {
      // Apply the change, noting how many keys a new selection names.
      let keys: number | null = null
      updateList((state) => {
        const next = update(state)
        if (next.selectedIds !== state.selectedIds) {
          keys = next.selectedIds?.length ?? 0
        }
        return next
      })
      if (keys === null) return
      paneEvent({ type: 'select', keys })
      if (keys === 1) setMutationError(null)
    },
    [updateList],
  )
  const tableState = useMemo<ListStateHandle>(
    () => [list, updateSelection],
    [list, updateSelection],
  )

  // select makes key the one selected row, or clears the selection.
  const select = useCallback(
    (key: string | null) =>
      updateSelection((state) => ({
        ...state,
        selectedIds: key === null ? [] : [key],
      })),
    [updateSelection],
  )

  // rows are the keys read, none until the first read arrives.
  const rows = useMemo(() => keysResource.value ?? [], [keysResource.value])

  // arrived is true once a read of the keys has succeeded; New Key shows only
  // then, as a viewer's presses act on a record that has arrived.
  const arrived =
    !keysResource.error && !(keysResource.loading && rows.length === 0)

  // The keys the prefix leaves, as table rows; the table sorts them.
  const visibleRows = useMemo(
    () =>
      rows
        .filter((row) => row.label.startsWith(prefix))
        .map((row) => ({ id: row.label, data: row })),
    [rows, prefix],
  )

  // selectedRow is the one selected key while the prefix leaves it.
  const selectedRow = useMemo(
    () => visibleRows.find((row) => row.id === selectedKey)?.data ?? null,
    [visibleRows, selectedKey],
  )

  // The write error the pane shows: the selected key's, or the create's when
  // no key is selected.
  const errorKey = selectedRow?.label ?? null
  const shownError =
    mutationError?.key === errorKey ? mutationError.message : null

  // handleStartCreate clears the selection and opens the create form.
  const handleStartCreate = useCallback(() => {
    select(null)
    setMutationError(null)
    paneEvent({ type: 'create' })
  }, [select])

  // runMutation applies a write for key, null for a create, then reloads the
  // key list so the watched list reconciles. A failure surfaces inline beside
  // that key and returns false; the reload rolls back any optimistic list
  // state that did not actually persist. It never throws, so callers finalize
  // selection only on success.
  const runMutation = useCallback(
    async (
      key: string | null,
      mutate: (kv: KvStore) => Promise<void>,
    ): Promise<boolean> => {
      // Run the write, keeping its failure for the pane.
      const kv = handle.value
      if (!kv) return false
      setMutationError(null)
      let ok = false
      try {
        await mutate(kv)
        ok = true
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err)
        setMutationError({ key, message })
      }
      keysResource.retry()
      return ok
    },
    [handle.value, keysResource],
  )

  // handleCreate writes a new key and selects it, which closes the form.
  const handleCreate = useCallback(
    async (key: Uint8Array, value: Uint8Array) => {
      const ok = await runMutation(null, (kv) =>
        kv.withTransaction(true, async (tx) => {
          await tx.set(key, value)
        }),
      )
      if (!ok) return
      select(bytesToText(key))
    },
    [runMutation, select],
  )

  // handleSave writes the selected key's new value.
  const handleSave = useCallback(
    async (value: Uint8Array) => {
      const row = selectedRow
      if (!row) return
      await runMutation(row.label, (kv) =>
        kv.withTransaction(true, async (tx) => {
          await tx.set(row.key, value)
        }),
      )
    },
    [runMutation, selectedRow],
  )

  // handleDelete deletes the selected key and, once it is gone, clears the
  // selection.
  const handleDelete = useCallback(async () => {
    const row = selectedRow
    if (!row) return
    const ok = await runMutation(row.label, (kv) =>
      kv.withTransaction(true, async (tx) => {
        await tx.delete(row.key)
      }),
    )
    if (ok) select(null)
  }, [runMutation, select, selectedRow])

  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <ViewerBar
        icon={<LuDatabase />}
        title="Key/Value Store"
        description={`${rows.length} ${rows.length === 1 ? 'key' : 'keys'}`}
        actions={
          arrived
            ? [
                {
                  id: 'new-key',
                  label: 'New Key',
                  icon: <LuPlus />,
                  onClick: handleStartCreate,
                },
              ]
            : []
        }
      />

      <div className="flex min-h-0 flex-1">
        <div className="border-foreground/8 flex w-1/3 min-w-48 flex-col border-r">
          <KvKeyListToolbar prefix={prefix} onPrefixChange={setPrefix} />
          {keysResource.error ? (
            <div className="min-h-0 flex-1">
              <ViewerStatus
                state="failed"
                message={`Could not read the keys: ${keysResource.error.message}`}
                retry={keysResource.retry}
              />
            </div>
          ) : keysResource.loading && rows.length === 0 ? (
            <div className="min-h-0 flex-1">
              <ViewerStatus state="waiting" what="the keys" rows={4} />
            </div>
          ) : (
            <KvKeyList
              rows={visibleRows}
              state={tableState}
              filtered={prefix.length > 0}
            />
          )}
        </div>

        <div className="min-w-0 flex-1">
          {selectedRow ? (
            <KvKeyDetail
              handle={handle}
              row={selectedRow}
              onSave={handleSave}
              onDelete={handleDelete}
              mutationError={shownError}
            />
          ) : creating ? (
            <div className="viewer-inset overflow-auto">
              <KvCreateKeyForm
                onCreate={handleCreate}
                onCancel={() => paneEvent({ type: 'close' })}
                mutationError={shownError}
              />
            </div>
          ) : (
            <div className="text-foreground-alt/40 flex h-full items-center justify-center text-xs">
              Select a key to view its value.
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
