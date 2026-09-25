import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { TransformConfigDisplay } from '@s4wave/web/transform/TransformConfigDisplay.js'

import { SpaceStorageCard } from './SpaceStorageCard.js'

// SpaceDataSection renders where the Space stores its data and its transform
// pipeline for the Data section.
export function SpaceDataSection() {
  const { spaceState } = SpaceContainerContext.useContext()
  const info = spaceState.transformInfo
  return (
    <div className="space-y-3">
      <SpaceStorageCard />
      {info && <TransformConfigDisplay info={info} />}
    </div>
  )
}
