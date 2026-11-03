import * as stylex from '@stylexjs/stylex'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { TransformConfigDisplay } from '@s4wave/web/transform/TransformConfigDisplay.js'

import { SpaceStorageCard } from './SpaceStorageCard.js'

// SpaceDataSection renders where the Space stores its data and its transform
// pipeline for the Data section.
export function SpaceDataSection({
  sx,
}: { sx?: stylex.StyleXStyles; className?: never } = {}) {
  const { spaceState } = SpaceContainerContext.useContext()
  const info = spaceState.transformInfo
  return (
    <div sx={sx}>
      <SpaceStorageCard sx={nativeChildSpacing.s3} />
      {info && (
        <TransformConfigDisplay sx={nativeChildSpacing.s3} info={info} />
      )}
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})
