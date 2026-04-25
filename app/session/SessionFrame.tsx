import React from 'react'

import {
  ViewerFrame,
  type ViewerFrameProps,
} from '@s4wave/web/frame/ViewerFrame.js'

export interface ISessionFrameProps extends ViewerFrameProps {}

// SessionFrame renders the session-level frame with bottom bar items.
export function SessionFrame(props: ISessionFrameProps) {
  return <ViewerFrame {...props} />
}
