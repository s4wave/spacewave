import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'

import { ForgeClusterViewerView } from './ForgeClusterViewerView.js'
import { useForgeClusterViewerController } from './useForgeClusterViewerController.js'

export { ForgeClusterTypeID } from './useForgeClusterViewerController.js'

export function ForgeClusterViewer(props: ObjectViewerComponentProps) {
  const controller = useForgeClusterViewerController(props)
  return <ForgeClusterViewerView controller={controller} />
}
