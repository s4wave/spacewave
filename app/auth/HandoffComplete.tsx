import { useCallback } from 'react'
import { LuArrowRight, LuCheck, LuLaptop, LuUser } from 'react-icons/lu'

import type { HandoffRequest } from '@s4wave/core/session/handoff/handoff.pb.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { clientTypeLabel } from './handoff-state.js'

// detailChipClassName styles one handoff detail chip.
const detailChipClassName =
  'border-foreground/10 bg-foreground/5 text-foreground-alt inline-flex items-center gap-1.5 rounded-full border px-3 py-1 whitespace-nowrap'

// HandoffDetails renders the account and device of a handoff as chips that
// wrap whole, so a long username or host name never splits across lines.
export function HandoffDetails({
  username,
  deviceName,
}: {
  username?: string
  deviceName: string
}) {
  if (!username && !deviceName) return null
  return (
    <div className="flex flex-wrap items-center justify-center gap-2 text-sm">
      {username && (
        <span className={detailChipClassName}>
          <LuUser className="size-3.5" />
          <span className="text-foreground font-medium">{username}</span>
        </span>
      )}
      {deviceName && (
        <span className={detailChipClassName}>
          <LuLaptop className="size-3.5" />
          {deviceName}
        </span>
      )}
    </div>
  )
}

// HandoffComplete confirms that a CLI or desktop handoff finished and offers
// the browser Session the person signed in to along the way.
export function HandoffComplete({
  request,
  sessionIndex,
}: {
  request: HandoffRequest
  sessionIndex: number
}) {
  const navigate = useNavigate()
  const label = clientTypeLabel(request.clientType ?? '')
  const handleOpen = useCallback(() => {
    navigate({ path: `/u/${sessionIndex}` })
  }, [navigate, sessionIndex])

  return (
    <div className="bg-background-landing relative flex flex-1 flex-col items-center justify-center gap-6 p-6">
      <div className="relative z-10 flex max-w-sm flex-col items-center gap-4 text-center">
        <div className="bg-brand/10 border-brand/30 flex size-16 items-center justify-center rounded-full border">
          <LuCheck className="text-brand size-8" />
        </div>
        <h1 className="text-xl font-semibold tracking-wide">
          Sign-in complete
        </h1>
        <HandoffDetails deviceName={request.deviceName ?? ''} />
        <p className="text-foreground-alt text-sm">
          Spacewave {label} is signed in. Return to it, or keep going here in
          the browser.
        </p>
        <button
          type="button"
          onClick={handleOpen}
          className="bg-brand text-brand-foreground hover:bg-brand/90 mt-2 flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
        >
          Open Spacewave
          <LuArrowRight className="size-4" />
        </button>
      </div>
    </div>
  )
}
