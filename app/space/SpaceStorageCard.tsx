import { LuDatabase } from 'react-icons/lu'

import { SpaceStorageMove } from '@s4wave/app/session/storage/SpaceStorageMove.js'
import { SpaceUploadStatus } from '@s4wave/app/session/storage/SpaceUploadStatus.js'
import {
  formatStorageLocation,
  guessStoragePreset,
} from '@s4wave/app/session/storage/storage-backend.js'
import { useStorageBackends } from '@s4wave/app/session/storage/useStorageBackends.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'

// SpaceStorageCard shows where the Space stores its data and moves it to
// another storage. It shows only for accounts with storage backends.
export function SpaceStorageCard() {
  const { spaceId } = SpaceContainerContext.useContext()
  const resource = useStorageBackends()
  const backends = resource.value?.storageBackends ?? []
  if (resource.error || backends.length === 0) {
    return null
  }

  const placed = backends.find((b) =>
    b.placedSpaces?.some((space) => space.spaceId === spaceId),
  )?.backend
  return (
    <div className="flex items-start gap-2 text-xs" data-testid="space-storage">
      <LuDatabase
        className="text-foreground-alt mt-0.5 size-3.5 shrink-0"
        aria-hidden="true"
      />
      <div className="min-w-0 space-y-1.5">
        <div className="text-foreground">
          {placed
            ? `Stored in ${placed.displayName}`
            : "Stored in the account's own storage"}
        </div>
        {placed && (
          <>
            <p className="text-foreground-alt/60 truncate">
              {formatStorageLocation(placed.s3)}
            </p>
            <SpaceUploadStatus
              sharedObjectId={spaceId}
              preset={guessStoragePreset(placed.s3)}
            />
          </>
        )}
        <SpaceStorageMove
          sharedObjectId={spaceId}
          placedId={placed?.id ?? ''}
          backends={backends}
        />
      </div>
    </div>
  )
}
