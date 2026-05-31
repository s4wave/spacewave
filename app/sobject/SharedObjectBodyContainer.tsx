import React from 'react'

import { CDN_BODY_TYPE, SPACE_BODY_TYPE } from '@s4wave/app/space/const.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SharedObjectContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainer } from '@s4wave/app/space/SpaceContainer.js'

function logQuickstartBodyTypeDecision(fields: Record<string, unknown>): void {
  if (
    !(globalThis as { __s4waveLogQuickstartTiming?: boolean })
      .__s4waveLogQuickstartTiming
  ) {
    return
  }
  console.log(
    'quickstart shared object body type decision: ' + JSON.stringify(fields),
  )
}

// SharedObjectBodyContainer renders the type-specific body of a SharedObject.
export function SharedObjectBodyContainer() {
  const sharedObject = useResourceValue(SharedObjectContext.useContext())
  const bodyType = sharedObject?.meta?.sharedObjectMeta?.bodyType
  logQuickstartBodyTypeDecision({
    sharedObjectId: sharedObject?.meta?.sharedObjectId ?? null,
    resourceId: sharedObject?.id ?? null,
    bodyType: bodyType ?? null,
    hasMeta: !!sharedObject?.meta,
    hasSharedObjectMeta: !!sharedObject?.meta?.sharedObjectMeta,
  })
  switch (bodyType) {
    case SPACE_BODY_TYPE:
    case CDN_BODY_TYPE:
      return <SpaceContainer />
    default:
      return <div>Unknown shared object body type: {bodyType}</div>
  }
}
