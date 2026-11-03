import { useState } from 'react'
import {
  LuPlay,
  LuRotateCcw,
  LuSlidersHorizontal,
  LuTv,
} from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { VmState, type VmMount } from '@s4wave/sdk/vm/v86.pb.js'

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
    width: '100%',
    maxWidth: 'var(--container-md)',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    display: 'flex',
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
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule5: {
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
  rule6: { minWidth: 0, flex: 1 },
  rule7: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: 'var(--text-base)',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule8: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule9: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule10: { display: 'flex', gap: `calc(${themeLayout['--spacing']} * 4)` },
  rule11: {
    width: `calc(${themeLayout['--spacing']} * 16)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
  },
  rule12: {
    minWidth: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
  },
  rule13: { color: themeColors['--color-foreground'] },
  rule14: { minWidth: 0, color: themeColors['--color-foreground'] },
  rule15: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * .5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule16: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
  },
  rule17: { color: themeColors['--color-foreground-alt'] },
  rule18: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-brand'],
  },
  rule19: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule20: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule21: {
    width: `calc(${themeLayout['--spacing']} * 28)`,
    flexShrink: 0,
    color: themeColors['--color-foreground'],
  },
  rule22: {
    height: `calc(${themeLayout['--spacing']} * 7)`,
    minWidth: 0,
    flex: 1,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

// OverrideSlot is a boot asset a VM may take from a UnixFS object instead
// of its V86Image.
export type OverrideSlot = 'kernel' | 'rootfs' | 'bios' | 'wasm'

// overrideSlots lists the override slots with their labels in display order.
const overrideSlots: { slot: OverrideSlot; label: string }[] = [
  { slot: 'kernel', label: 'Kernel' },
  { slot: 'rootfs', label: 'Root filesystem' },
  { slot: 'bios', label: 'BIOS' },
  { slot: 'wasm', label: 'Emulator' },
]

// VmMachineCardProps describes a stopped or failed VM and its actions.
export interface VmMachineCardProps {
  name: string
  state: VmState
  errorMessage: string
  // operationError is the failure of the last request from this view.
  operationError: string
  imageKey: string
  // imageLabel names the image for display, or is "" to show imageKey.
  imageLabel: string
  memoryMb: number
  mounts: VmMount[]
  // startDisabled holds Start until the Space can bind the VM's process.
  startDisabled: boolean
  onStart: () => void
  onReset: () => void
  // overrides maps each slot to its override object key, "" for the image's
  // asset, or is null while loading.
  overrides: Record<OverrideSlot, string> | null
  unixfsKeys: string[]
  // onOverrideChange moves a slot's override from prevKey to nextKey.
  onOverrideChange: (
    slot: OverrideSlot,
    prevKey: string,
    nextKey: string,
  ) => void
}

// VmMachineCard shows a VM that is not running: what it boots and the
// action that starts it, or the error that stopped it and the reset.
export function VmMachineCard({
  name,
  state,
  errorMessage,
  operationError,
  imageKey,
  imageLabel,
  memoryMb,
  mounts,
  startDisabled,
  onStart,
  onReset,
  overrides,
  unixfsKeys,
  onOverrideChange,
}: VmMachineCardProps) {
  const failed = state === VmState.VmState_ERROR
  const [overridesOpen, setOverridesOpen] = useState(false)
  const overrideCount = overrides
    ? Object.values(overrides).filter(Boolean).length
    : 0

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <section sx={stylexStyles2.rule3}>
          {/* Identity */}
          <div sx={stylexStyles2.rule4}>
            <div sx={stylexStyles2.rule5}>
              <LuTv sx={stylexStyles.rule1} />
            </div>
            <div sx={stylexStyles2.rule6}>
              <h2 sx={stylexStyles2.rule7}>{name}</h2>
              <div sx={stylexStyles2.rule8}>x86 virtual machine</div>
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
          <dl sx={stylexStyles2.rule9}>
            <div sx={stylexStyles2.rule10}>
              <dt sx={stylexStyles2.rule11}>Image</dt>
              <dd sx={stylexStyles2.rule12} title={imageKey}>
                {imageLabel || imageKey || 'None'}
              </dd>
            </div>
            <div sx={stylexStyles2.rule10}>
              <dt sx={stylexStyles2.rule11}>Memory</dt>
              <dd sx={stylexStyles2.rule13}>
                {memoryMb ? `${memoryMb} MB` : 'Default'}
              </dd>
            </div>
            <div sx={stylexStyles2.rule10}>
              <dt sx={stylexStyles2.rule11}>Mounts</dt>
              <dd sx={stylexStyles2.rule14}>
                {mounts.length === 0 ? (
                  'None'
                ) : (
                  <ul sx={stylexStyles2.rule15}>
                    {mounts.map((m) => (
                      <li
                        key={`${m.path ?? ''}:${m.objectKey ?? ''}`}
                        sx={stylexStyles2.rule16}
                        title={m.objectKey}
                      >
                        {m.path || '/'}{' '}
                        <span sx={stylexStyles2.rule17}>
                          {m.writable ? 'rw' : 'ro'}
                        </span>
                      </li>
                    ))}
                  </ul>
                )}
              </dd>
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

        <CollapsibleSection
          title="Asset overrides"
          icon={<LuSlidersHorizontal />}
          open={overridesOpen}
          onOpenChange={setOverridesOpen}
          badge={
            overrideCount > 0 && (
              <span sx={stylexStyles2.rule18}>{overrideCount} set</span>
            )
          }
        >
          {!overrides ? (
            <div sx={stylexStyles2.rule8}>Loading</div>
          ) : (
            <div sx={stylexStyles2.rule19}>
              <p sx={stylexStyles2.rule8}>
                Boot from a file in this Space instead of the image's asset.
              </p>
              {overrideSlots.map(({ slot, label }) => (
                <label key={slot} sx={stylexStyles2.rule20}>
                  <span sx={stylexStyles2.rule21}>{label}</span>
                  <select
                    value={overrides[slot]}
                    onChange={(e) =>
                      onOverrideChange(slot, overrides[slot], e.target.value)
                    }
                    sx={stylexStyles2.rule22}
                  >
                    <option value="">From image</option>
                    {overrides[slot] &&
                      !unixfsKeys.includes(overrides[slot]) && (
                        <option value={overrides[slot]}>
                          {overrides[slot]}
                        </option>
                      )}
                    {unixfsKeys.map((key) => (
                      <option key={key} value={key}>
                        {key}
                      </option>
                    ))}
                  </select>
                </label>
              ))}
            </div>
          )}
        </CollapsibleSection>
      </div>
    </div>
  )
}
