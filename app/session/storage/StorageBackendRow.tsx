import { useCallback, useState } from 'react'
import { LuPlugZap, LuStar, LuStarOff, LuTrash2 } from 'react-icons/lu'
import { isDesktop } from '@aptre/bldr'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

import type { StorageBackendInfo } from '@s4wave/sdk/session/session.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { SpaceUploadStatus } from './SpaceUploadStatus.js'
import { StorageCheckResult } from './StorageCheckResult.js'
import {
  buildStorageCorsRule,
  describeStorageCheck,
  formatStorageLocation,
  guessStoragePreset,
  type StorageCheckView,
} from './storage-backend.js'

interface StorageBackendRowProps {
  info: StorageBackendInfo
  isDefault: boolean
}

// StorageBackendRow shows one storage backend, the upload state of each
// Space placed on it, and its check, default, and remove actions.
export function StorageBackendRow({ info, isDefault }: StorageBackendRowProps) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const [check, setCheck] = useState<StorageCheckView | null>(null)
  const [busy, setBusy] = useState(false)
  const [confirmRemove, setConfirmRemove] = useState(false)

  const backend = info.backend
  const id = backend?.id ?? ''
  const name = backend?.displayName ?? ''
  const preset = guessStoragePreset(backend?.s3)
  const placed = info.placedSpaces ?? []

  // Run one action at a time. A failure is reported and returns undefined.
  const run = useCallback(async <T,>(action: () => Promise<T>) => {
    setBusy(true)
    try {
      return await action()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err))
      return undefined
    } finally {
      setBusy(false)
    }
  }, [])

  const handleCheck = useCallback(async () => {
    if (!session) return
    const resp = await run(() =>
      session.checkStorageBackend({ storageBackendId: id }),
    )
    if (resp) {
      setCheck(describeStorageCheck(resp.result, !isDesktop))
    }
  }, [id, run, session])

  const handleDefault = useCallback(() => {
    if (!session) return
    void run(() => session.setDefaultStorageBackend(isDefault ? '' : id))
  }, [id, isDefault, run, session])

  const handleRemove = useCallback(() => {
    if (!session) return
    if (!confirmRemove) {
      setConfirmRemove(true)
      return
    }
    void run(() => session.removeStorageBackend(id))
  }, [confirmRemove, id, run, session])

  return (
    <div
      className="py-3 first:pt-0 last:pb-0"
      data-testid="storage-backend-row"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="text-foreground truncate text-xs font-medium">
              {name}
            </h3>
            {isDefault && (
              <span className="bg-brand/15 text-brand rounded px-1.5 py-0.5 text-xs">
                New Spaces
              </span>
            )}
          </div>
          <p className="text-foreground-alt/60 mt-0.5 truncate text-xs">
            {formatStorageLocation(backend?.s3)}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <DashboardButton
            icon={<LuPlugZap className="size-3.5" />}
            onClick={() => void handleCheck()}
            disabled={busy}
          >
            Check
          </DashboardButton>
          <DashboardButton
            icon={
              isDefault ? (
                <LuStarOff className="size-3.5" />
              ) : (
                <LuStar className="size-3.5" />
              )
            }
            onClick={handleDefault}
            disabled={busy}
            title={
              isDefault
                ? "New Spaces use the account's own storage"
                : 'New Spaces store their data in this bucket'
            }
          >
            {isDefault ? 'Stop using for new' : 'Use for new'}
          </DashboardButton>
          <DashboardButton
            icon={<LuTrash2 className="size-3.5" />}
            variant="destructive"
            onClick={handleRemove}
            onBlur={() => setConfirmRemove(false)}
            disabled={busy || placed.length !== 0}
            title={
              placed.length !== 0
                ? 'Move or delete the Spaces stored here first'
                : undefined
            }
          >
            {confirmRemove ? 'Confirm remove' : 'Remove'}
          </DashboardButton>
        </div>
      </div>

      {check && (
        <div className="mt-2">
          <StorageCheckResult
            check={check}
            corsRule={buildStorageCorsRule(preset, window.location.origin)}
          />
        </div>
      )}

      {placed.length === 0 ? (
        <p className="text-foreground-alt/60 mt-2 text-xs">No Spaces yet.</p>
      ) : (
        <ul className="mt-2 space-y-2">
          {placed.map((space) => (
            <li key={space.spaceId}>
              <div className="text-foreground text-xs">
                {space.name || space.spaceId}
              </div>
              <SpaceUploadStatus
                sharedObjectId={space.spaceId ?? ''}
                preset={preset}
              />
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
