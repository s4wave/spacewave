import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'

import { ForgeDashboardViewerView } from './ForgeDashboardViewerView.js'
import { useForgeDashboardViewerController } from './useForgeDashboardViewerController.js'

export { ForgeDashboardTypeID } from './useForgeDashboardViewerController.js'

export function ForgeDashboardViewer(props: ObjectViewerComponentProps) {
  const controller = useForgeDashboardViewerController(props)
  return <ForgeDashboardViewerView controller={controller} />
}
