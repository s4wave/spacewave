import { LuCheck, LuX } from 'react-icons/lu'

import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'

// CheckoutResultPage renders a static checkout result for desktop app users.
// When the desktop app opens Stripe in an external browser, Stripe redirects
// here after completion. The desktop app receives status via streaming RPC.
export function CheckoutResultPage({ success }: { success?: boolean }) {
  return (
    <div className="bg-background-landing relative flex flex-1 flex-col items-center justify-center p-6">
      <ShootingStars className="pointer-events-none fixed inset-0 opacity-60" />
      <div className="relative z-10 flex flex-col items-center gap-6 text-center">
        <AnimatedLogo followMouse={false} />
        {success ?
          <>
            <div className="bg-brand/10 flex h-16 w-16 items-center justify-center rounded-full">
              <LuCheck className="text-brand h-8 w-8" />
            </div>
            <h1 className="text-foreground text-xl font-bold">
              Subscription activated!
            </h1>
            <p className="text-foreground-alt text-sm">
              You can close this tab and return to the app.
            </p>
          </>
        : <>
            <div className="bg-destructive/10 flex h-16 w-16 items-center justify-center rounded-full">
              <LuX className="text-destructive h-8 w-8" />
            </div>
            <h1 className="text-foreground text-xl font-bold">
              Checkout canceled
            </h1>
            <p className="text-foreground-alt text-sm">
              You can close this tab and return to the app to try again.
            </p>
          </>
        }
      </div>
    </div>
  )
}
