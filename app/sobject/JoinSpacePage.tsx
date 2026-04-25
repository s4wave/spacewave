import { useCallback } from 'react'

import { useNavigate, useParams } from '@s4wave/web/router/router.js'

import { JoinSpaceDialog } from './JoinSpaceDialog.js'

// JoinSpacePage is a route wrapper that renders JoinSpaceDialog always-open.
// When the dialog closes, navigates back to the session dashboard.
// Mounted at /join and /join/:code.
export function JoinSpacePage() {
  const params = useParams()
  const navigate = useNavigate()
  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (!open) navigate({ path: '../' })
    },
    [navigate],
  )
  return (
    <JoinSpaceDialog
      open
      onOpenChange={handleOpenChange}
      initialCode={params.code}
    />
  )
}
