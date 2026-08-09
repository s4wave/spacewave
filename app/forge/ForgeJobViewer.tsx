import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'

import { ForgeJobViewerView } from './ForgeJobViewerView.js'
import { useForgeJobViewerController } from './useForgeJobViewerController.js'

export { ForgeJobTypeID } from './useForgeJobViewerController.js'

export function ForgeJobViewer(props: ObjectViewerComponentProps) {
  const controller = useForgeJobViewerController(props)
  return <ForgeJobViewerView controller={controller} />
}
