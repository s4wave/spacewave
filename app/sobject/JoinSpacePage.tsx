import { useCallback } from 'react'

import { useNavigate, useParams } from '@s4wave/web/router/router.js'

import { JoinSpaceDialog } from './JoinSpaceDialog.js'

// JoinSpacePage is a route wrapper that renders JoinSpaceDialog always-open.
// Accepted joins navigate to the exact shared Space returned by the session.
export function JoinSpacePage() {
  const params = useParams()
  const navigate = useNavigate()
  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (!open) navigate({ path: '../' })
    },
    [navigate],
  )
  const handleAccepted = useCallback(
    (sharedObjectId: string) => {
      navigate({
        path: `../so/${encodeURIComponent(sharedObjectId)}`,
        replace: true,
      })
    },
    [navigate],
  )
  return (
    <JoinSpaceDialog
      open
      onOpenChange={handleOpenChange}
      onAccepted={handleAccepted}
      initialCode={params.code}
    />
  )
}
