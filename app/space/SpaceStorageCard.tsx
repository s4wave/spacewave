import { LuDatabase } from '@s4wave/web/images/icons.js'

import { SpaceStorageMove } from '@s4wave/app/session/storage/SpaceStorageMove.js'
import { SpaceUploadStatus } from '@s4wave/app/session/storage/SpaceUploadStatus.js'
import {
  formatStorageLocation,
  guessStoragePreset,
} from '@s4wave/app/session/storage/storage-backend.js'
import { useStorageBackends } from '@s4wave/app/session/storage/useStorageBackends.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
  },
})

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
      <LuDatabase sx={stylexStyles.rule1} aria-hidden="true" />
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
