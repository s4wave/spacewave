import { useCallback, useEffect, type ReactNode } from 'react'

import { useHistory } from '@s4wave/web/router/HistoryRouter.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'

import { SessionFrame } from './SessionFrame.js'

// SessionFlowFrameProps configures the session subflow frame.
export interface SessionFlowFrameProps {
  fallbackPath: string
  children: ReactNode
}

// SessionFlowFrame keeps session subflows inside the session chrome while
// providing the same top-left back action used by auth screens.
export function SessionFlowFrame({
  fallbackPath,
  children,
}: SessionFlowFrameProps) {
  const history = useHistory()
  const navigate = useNavigate()
  useEffect(() => {
    history?.enterFlow(fallbackPath)
  }, [fallbackPath, history])

  const handleBack = useCallback(() => {
    if (history) {
      history.exitFlow(fallbackPath)
      return
    }
    navigate({ path: fallbackPath })
  }, [fallbackPath, history, navigate])

  return (
    <SessionFrame>
      <div className="relative flex min-h-0 flex-1 overflow-hidden">
        <BackButton floating onClick={handleBack}>
          Back
        </BackButton>
        {children}
      </div>
    </SessionFrame>
  )
}
