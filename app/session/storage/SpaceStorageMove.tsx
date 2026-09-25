import { useCallback, useEffect, useId, useRef, useState } from 'react'
import { LuArrowRightLeft } from 'react-icons/lu'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

import {
  MoveSpaceStoragePhase,
  type StorageBackendInfo,
} from '@s4wave/sdk/session/session.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import {
  accountStorageChoice,
  describeMoveProgress,
} from './storage-backend.js'

interface SpaceStorageMoveProps {
  sharedObjectId: string
  // placedId is the backend holding the Space, empty for the account's own storage.
  placedId: string
  backends: StorageBackendInfo[]
}

// SpaceStorageMove moves a Space to another storage backend and shows the
// progress. Leaving the page stops the progress stream; the upload continues.
export function SpaceStorageMove({
  sharedObjectId,
  placedId,
  backends,
}: SpaceStorageMoveProps) {
  const id = useId()
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const [target, setTarget] = useState('')
  const [progress, setProgress] = useState('')
  const [moving, setMoving] = useState(false)
  const abortRef = useRef<AbortController | null>(null)
  useEffect(() => () => abortRef.current?.abort(), [])

  const destinations = [
    ...(placedId
      ? [{ id: accountStorageChoice, name: "the account's own storage" }]
      : []),
    ...backends
      .filter((b) => b.backend?.id && b.backend.id !== placedId)
      .map((b) => ({
        id: b.backend?.id ?? '',
        name: b.backend?.displayName ?? '',
      })),
  ]
  const selected = destinations.find((d) => d.id === target) ?? destinations[0]

  const handleMove = useCallback(async () => {
    if (!session || !selected) return
    const abort = new AbortController()
    abortRef.current = abort
    const backendId = selected.id === accountStorageChoice ? '' : selected.id
    setMoving(true)
    try {
      for await (const step of session.moveSpaceStorage(
        sharedObjectId,
        backendId,
        abort.signal,
      )) {
        setProgress(describeMoveProgress(step, selected.name))
        if (step.phase === MoveSpaceStoragePhase.MoveSpaceStoragePhase_DONE) {
          toast.success(`Moved to ${selected.name}`)
        }
      }
    } catch (err) {
      if (!abort.signal.aborted) {
        toast.error(err instanceof Error ? err.message : String(err))
      }
    } finally {
      setMoving(false)
      setProgress('')
    }
  }, [session, selected, sharedObjectId])

  if (!selected) {
    return null
  }
  if (moving) {
    return (
      <p
        className="text-foreground-alt text-xs"
        data-testid="space-storage-move-progress"
      >
        {progress || `Moving to ${selected.name}`}
      </p>
    )
  }
  return (
    <div className="flex items-center gap-2 text-xs">
      <label htmlFor={id} className="text-foreground-alt select-none">
        Move to
      </label>
      <select
        id={id}
        value={selected.id}
        onChange={(e) => setTarget(e.target.value)}
        className="border-foreground/15 bg-background/40 text-foreground focus:border-brand/50 min-w-0 rounded-md border px-2 py-1 text-xs outline-none"
        data-testid="space-storage-move-target"
      >
        {destinations.map((d) => (
          <option key={d.id} value={d.id}>
            {d.name}
          </option>
        ))}
      </select>
      <DashboardButton
        icon={<LuArrowRightLeft className="size-3.5" />}
        onClick={() => void handleMove()}
      >
        Move
      </DashboardButton>
    </div>
  )
}
