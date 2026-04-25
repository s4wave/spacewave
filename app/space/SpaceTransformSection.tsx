import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { TransformConfigDisplay } from '@s4wave/web/transform/TransformConfigDisplay.js'

// SpaceDataSection renders transform pipeline info for the Data section.
// Reads transform info from the space state via SpaceContainerContext.
export function SpaceDataSection() {
  const { spaceState } = SpaceContainerContext.useContext()
  const info = spaceState.transformInfo
  if (!info) return null
  return <TransformConfigDisplay info={info} />
}
