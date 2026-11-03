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

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({ rule1: { minWidth: 0 } })

const stylexStyles2 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'flex-start',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: { color: themeColors['--color-foreground'] },
  rule3: {
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-foreground-alt'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// SpaceStorageCard shows where the Space stores its data and moves it to
// another storage. It shows only for accounts with storage backends.
export function SpaceStorageCard({
  sx,
}: { sx?: stylex.StyleXStyles; className?: never } = {}) {
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
    <div sx={[stylexStyles2.rule1, sx]} data-testid="space-storage">
      <LuDatabase sx={stylexStyles.rule1} aria-hidden="true" />
      <div sx={stylexStyles3.rule1}>
        <div sx={[nativeChildSpacing.s1_5, stylexStyles2.rule2]}>
          {placed
            ? `Stored in ${placed.displayName}`
            : "Stored in the account's own storage"}
        </div>
        {placed && (
          <>
            <p sx={[nativeChildSpacing.s1_5, stylexStyles2.rule3]}>
              {formatStorageLocation(placed.s3)}
            </p>
            <SpaceUploadStatus
              sx={nativeChildSpacing.s1_5}
              sharedObjectId={spaceId}
              preset={guessStoragePreset(placed.s3)}
            />
          </>
        )}
        <SpaceStorageMove
          sx={nativeChildSpacing.s1_5}
          sharedObjectId={spaceId}
          placedId={placed?.id ?? ''}
          backends={backends}
        />
      </div>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
  },
})
