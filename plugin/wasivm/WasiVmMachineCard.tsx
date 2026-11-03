import { LuPlay, LuRotateCcw, LuTerminal } from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'
import { VmState } from '@s4wave/sdk/vm/v86.pb.js'

import { VmErrorBanner } from '@s4wave/app/vm/VmErrorBanner.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '@s4wave/web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '@s4wave/web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    padding: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    flex: 1,
    display: 'flex',
    justifyContent: 'center',
    minHeight: 0,
    overflowY: 'auto',
  },
  rule2: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    display: 'flex',
    flexDirection: 'column',
    height: 'fit-content',
    maxWidth: 'var(--container-md)',
    width: '100%',
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule4: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule5: { flex: 1, minWidth: 0, },
  rule6: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-base)',
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule7: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule8: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'flex',
    flexDirection: 'column',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule9: { gap: `calc(${themeLayout['--spacing']} * 4)`, display: 'flex', },
  rule10: {
    color: themeColors['--color-foreground-alt'],
    flexShrink: 0,
    width: `calc(${themeLayout['--spacing']} * 24)`,
  },
  rule11: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

// WasiVmMachineCardProps describes a stopped or failed WasiVM and its
// actions.
export interface WasiVmMachineCardProps {
  name: string
  state: VmState
  errorMessage: string
  // operationError is the failure of the last request from this view.
  operationError: string
  rootfsKey: string
  // init is the init command line, program first.
  init: string
  // startDisabled holds Start until the Space can bind the VM's process.
  startDisabled: boolean
  onStart: () => void
  onReset: () => void
}

// WasiVmMachineCard shows a WasiVM that is not running: what it runs and the
// action that starts it, or the error that stopped it and the reset.
export function WasiVmMachineCard({
  name,
  state,
  errorMessage,
  operationError,
  rootfsKey,
  init,
  startDisabled,
  onStart,
  onReset,
}: WasiVmMachineCardProps) {
  const failed = state === VmState.VmState_ERROR
  return (
    <div sx={stylexStyles2.rule1}>
      <section sx={stylexStyles2.rule2}>
        {/* Identity */}
        <div sx={stylexStyles2.rule3}>
          <div sx={stylexStyles2.rule4}>
            <LuTerminal sx={stylexStyles.rule1} />
          </div>
          <div sx={stylexStyles2.rule5}>
            <h2 sx={stylexStyles2.rule6}>{name}</h2>
            <div sx={stylexStyles2.rule7}>WebAssembly virtual machine</div>
          </div>
        </div>

        {failed && (
          <VmErrorBanner
            title="Runtime error"
            message={errorMessage || 'The VM stopped with an error.'}
          />
        )}
        {operationError && (
          <VmErrorBanner title="Operation error" message={operationError} />
        )}

        {/* Configuration */}
        <dl sx={stylexStyles2.rule8}>
          <div sx={stylexStyles2.rule9}>
            <dt sx={stylexStyles2.rule10}>Root filesystem</dt>
            <dd sx={stylexStyles2.rule11} title={rootfsKey}>
              {rootfsKey || 'None'}
            </dd>
          </div>
          <div sx={stylexStyles2.rule9}>
            <dt sx={stylexStyles2.rule10}>Init</dt>
            <dd sx={stylexStyles2.rule11}>{init}</dd>
          </div>
        </dl>

        {/* Action */}
        {failed ? (
          <Button variant="destructiveOutline" onClick={onReset}>
            <LuRotateCcw />
            Reset
          </Button>
        ) : (
          <Button disabled={startDisabled} onClick={onStart}>
            <LuPlay />
            Start
          </Button>
        )}
      </section>
    </div>
  )
}
