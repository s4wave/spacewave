import { LuCheck, LuX } from '@s4wave/web/images/icons.js'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    color: themeColors['--color-brand'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    color: themeColors['--color-destructive'],
  },
})

// CheckoutResultPage renders a static checkout result for desktop app users.
// When the desktop app opens Stripe in an external browser, Stripe redirects
// here after completion. The desktop app receives status via streaming RPC.
export function CheckoutResultPage({ success }: { success?: boolean }) {
  return (
    <div className="bg-background-landing relative flex flex-1 flex-col items-center justify-center p-6">
      <div className="relative z-10 flex flex-col items-center gap-6 text-center">
        <AnimatedLogo followMouse={false} />
        {success ? (
          <>
            <div className="bg-brand/10 flex size-16 items-center justify-center rounded-full">
              <LuCheck sx={stylexStyles.rule1} />
            </div>
            <h1 className="text-foreground text-xl font-semibold">
              Subscription activated!
            </h1>
            <p className="text-foreground-alt text-sm">
              You can close this tab and return to the app.
            </p>
          </>
        ) : (
          <>
            <div className="bg-destructive/10 flex size-16 items-center justify-center rounded-full">
              <LuX sx={stylexStyles.rule2} />
            </div>
            <h1 className="text-foreground text-xl font-semibold">
              Checkout canceled
            </h1>
            <p className="text-foreground-alt text-sm">
              You can close this tab and return to the app to try again.
            </p>
          </>
        )}
      </div>
    </div>
  )
}
