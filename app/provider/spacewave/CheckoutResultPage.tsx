import { LuCheck, LuX } from '@s4wave/web/images/icons.js'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import { fonts as themeFonts } from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: themeColors['--color-background-landing'],
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule2: {
    position: 'relative',
    zIndex: 10,
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    textAlign: 'center',
  },
  rule3: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 16)`,
    height: `calc(${themeLayout['--spacing']} * 16)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule4: {
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule5: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule6: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 16)`,
    height: `calc(${themeLayout['--spacing']} * 16)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
  },
})

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
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <AnimatedLogo followMouse={false} />
        {success ? (
          <>
            <div sx={stylexStyles2.rule3}>
              <LuCheck sx={stylexStyles.rule1} />
            </div>
            <h1 sx={stylexStyles2.rule4}>Subscription activated!</h1>
            <p sx={stylexStyles2.rule5}>
              You can close this tab and return to the app.
            </p>
          </>
        ) : (
          <>
            <div sx={stylexStyles2.rule6}>
              <LuX sx={stylexStyles.rule2} />
            </div>
            <h1 sx={stylexStyles2.rule4}>Checkout canceled</h1>
            <p sx={stylexStyles2.rule5}>
              You can close this tab and return to the app to try again.
            </p>
          </>
        )}
      </div>
    </div>
  )
}
