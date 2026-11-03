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
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    flex: 1,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-landing'],
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    position: 'relative',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    textAlign: 'center',
    zIndex: 10,
  },
  rule3: {
    borderRadius: '3.40282e38px',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 16)`,
    width: `calc(${themeLayout['--spacing']} * 16)`,
  },
  rule4: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
  },
  rule5: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule6: {
    borderRadius: '3.40282e38px',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 16)`,
    width: `calc(${themeLayout['--spacing']} * 16)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule2: {
    color: themeColors['--color-destructive'],
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
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
