import { LuPlay, LuRotateCcw, LuTerminal } from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'
import { VmState } from '@s4wave/sdk/vm/v86.pb.js'

import { VmErrorBanner } from '@s4wave/app/vm/VmErrorBanner.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    minHeight: 0,
    flex: 1,
    justifyContent: 'center',
    overflowY: 'auto',
    padding: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
  },
  rule2: {
    display: 'flex',
    height: 'fit-content',
    width: '100%',
    maxWidth: 'var(--container-md)',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule5: { minWidth: 0, flex: 1 },
  rule6: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: 'var(--text-base)',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule7: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule8: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule9: { display: 'flex', gap: `calc(${themeLayout['--spacing']} * 4)` },
  rule10: {
    width: `calc(${themeLayout['--spacing']} * 24)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
  },
  rule11: {
    minWidth: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: themeColors['--color-foreground'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
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
