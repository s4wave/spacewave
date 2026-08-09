import { LuCheck } from 'react-icons/lu'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

// PasskeyProgress renders the active passkey operation state.
export function PasskeyProgress({
  complete,
  message,
}: {
  complete: boolean
  message: string
}) {
  return (
    <div className="bg-background-landing relative flex flex-1 flex-col items-center justify-center p-6">
      <div className="relative z-10 flex flex-col items-center gap-4 text-center">
        <AnimatedLogo followMouse={false} />
        {complete ? (
          <LuCheck className="text-brand size-6" />
        ) : (
          <Spinner size="md" className="text-foreground-alt" />
        )}
        <p className="text-foreground-alt text-sm">{message}</p>
      </div>
    </div>
  )
}
