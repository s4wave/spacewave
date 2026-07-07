import React, { useCallback } from 'react'

import { useNavigate, useParams } from '@s4wave/web/router/router.js'
import { Quickstart } from '@s4wave/app/quickstart/Quickstart.js'
import { QuickstartUnavailable } from '@s4wave/app/quickstart/QuickstartUnavailable.js'
import { isQuickstartId } from '@s4wave/app/quickstart/options.js'

import './AppQuickstart.css'

export function AppQuickstart() {
  const quickstartId = useParams()['quickstartId']
  const navigate = useNavigate()
  const navigateHome = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])
  if (!quickstartId || !isQuickstartId(quickstartId)) {
    return (
      <QuickstartUnavailable
        quickstartId={quickstartId}
        onBack={navigateHome}
      />
    )
  }

  return <Quickstart quickstartId={quickstartId} />
}
