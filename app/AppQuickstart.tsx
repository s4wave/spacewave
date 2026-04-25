import React from 'react'

import { useParams } from '@s4wave/web/router/router.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'
import { Quickstart } from '@s4wave/app/quickstart/Quickstart.js'
import {
  QUICKSTART_OPTIONS,
  type QuickstartId,
} from '@s4wave/app/quickstart/options.js'

import './AppQuickstart.css'

function isQuickstartId(id: string): id is QuickstartId {
  return QUICKSTART_OPTIONS.some((opt) => opt.id === id)
}

export function AppQuickstart() {
  const quickstartId = useParams()['quickstartId']
  if (!quickstartId) {
    console.log('unknown quickstart id', { quickstartId })
    return <NavigatePath to={'/'} />
  }

  if (!isQuickstartId(quickstartId)) {
    console.log('invalid quickstart id', { quickstartId })
    return <NavigatePath to={'/'} />
  }

  return <Quickstart quickstartId={quickstartId} />
}
