import React from 'react'

import { CDN_BODY_TYPE, SPACE_BODY_TYPE } from '@s4wave/app/space/const.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SharedObjectContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainer } from '@s4wave/app/space/SpaceContainer.js'

// SharedObjectBodyContainer renders the type-specific body of a SharedObject.
export function SharedObjectBodyContainer() {
  const sharedObject = useResourceValue(SharedObjectContext.useContext())
  const bodyType = sharedObject?.meta?.sharedObjectMeta?.bodyType
  switch (bodyType) {
    case SPACE_BODY_TYPE:
    case CDN_BODY_TYPE:
      return <SpaceContainer />
    default:
      return <div>Unknown shared object body type: {bodyType}</div>
  }
}
