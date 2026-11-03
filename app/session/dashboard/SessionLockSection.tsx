import { useCallback, useId, useState } from 'react'
import { LuLock, LuLockOpen } from '@s4wave/web/images/icons.js'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import { SessionLockMode } from '@s4wave/core/session/session.pb.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  motion as themeMotion,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule3: { display: 'flex', gap: `calc(${themeLayout['--spacing']} * 2)` },
  rule4: {
    flex: 1,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule5: {
    flex: 1,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule7: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule8: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule9: {
    marginBottom: themeLayout['--spacing'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule10: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule11: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 50%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

export interface SessionLockSectionProps {
  // embedded hides the section heading and outer wrapper when rendered inside
  // a parent CollapsibleSection.
  embedded?: boolean
}

// SessionLockSection displays session lock mode controls.
export function SessionLockSection({ embedded }: SessionLockSectionProps) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const lockStateResource = useStreamingResource(
    sessionResource,
    (session, signal) => session.watchLockState({}, signal),
    [],
  )
  const lockState = lockStateResource.value
  const loading = !lockState

  const [selectedMode, setSelectedMode] = useState<'auto' | 'pin' | null>(null)
  const [changingPin, setChangingPin] = useState(false)
  const [pin, setPin] = useState('')
  const [confirmPin, setConfirmPin] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const currentMode =
    lockState?.mode === SessionLockMode.PIN_ENCRYPTED ? 'pin' : 'auto'
  const displayMode = selectedMode ?? currentMode

  const handleSave = useCallback(async () => {
    const pinError = displayMode === 'pin' ? validatePin(pin, confirmPin) : null
    if (pinError) {
      setError(pinError)
      return
    }
    setError(null)
    setSaving(true)
    try {
      const mode =
        displayMode === 'pin'
          ? SessionLockMode.PIN_ENCRYPTED
          : SessionLockMode.AUTO_UNLOCK
      const pinBytes =
        displayMode === 'pin' ? new TextEncoder().encode(pin) : undefined
      await session?.setLockMode(mode, pinBytes)
      setSelectedMode(null)
      setChangingPin(false)
      setPin('')
      setConfirmPin('')
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to change lock mode',
      )
    } finally {
      setSaving(false)
    }
  }, [session, displayMode, pin, confirmPin])

  function handleCancel() {
    setSelectedMode(null)
    setChangingPin(false)
    setPin('')
    setConfirmPin('')
    setError(null)
  }

  const hasChanges =
    changingPin || (selectedMode !== null && selectedMode !== currentMode)
  const isPinMode = displayMode === 'pin'
  const needsPin = isPinMode && hasChanges

  const content = (
    <>
      {loading && (
        <LoadingInline label="Loading lock state" tone="muted" size="sm" />
      )}
      {!loading && (
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
          }
        >
          <div>
            <RadioOption
              sx={nativeChildSpacingNext.s1_5}
              selected={displayMode === 'auto'}
              onSelect={() => setSelectedMode('auto')}
              icon={<LuLockOpen sx={stylexStyles.rule1} />}
              label="Auto-unlock"
              description="Key stored on disk. No PIN needed."
            />
            <RadioOption
              sx={nativeChildSpacingNext.s1_5}
              selected={displayMode === 'pin'}
              onSelect={() => setSelectedMode('pin')}
              icon={<LuLock sx={stylexStyles.rule1} />}
              label="PIN lock"
              description="Key encrypted with PIN. Enter PIN on each app launch."
            />
          </div>

          {needsPin && (
            <PinFields
              pin={pin}
              confirmPin={confirmPin}
              onPinChange={setPin}
              onConfirmPinChange={setConfirmPin}
            />
          )}

          {currentMode === 'pin' && !hasChanges && (
            <button
              type="button"
              onClick={() => setChangingPin(true)}
              sx={stylexStyles2.rule1}
            >
              Change PIN
            </button>
          )}

          {error && <p sx={stylexStyles2.rule2}>{error}</p>}

          {hasChanges && (
            <div sx={stylexStyles2.rule3}>
              <button
                type="button"
                onClick={() => void handleSave()}
                disabled={saving}
                sx={[stylexStyles2.rule4]}
              >
                {saving ? 'Saving…' : 'Save'}
              </button>
              <button
                type="button"
                onClick={handleCancel}
                sx={[stylexStyles2.rule5]}
              >
                Cancel
              </button>
            </div>
          )}

          <p sx={stylexStyles2.rule6}>
            Changing lock mode only requires your current session. No account
            re-auth needed.
          </p>
        </div>
      )}
    </>
  )

  if (embedded) return content

  return (
    <section>
      <div sx={stylexStyles2.rule7}>
        <h2 sx={stylexStyles2.rule8}>
          <LuLock sx={stylexStyles.rule2} />
          Session Lock
        </h2>
      </div>
      <InfoCard>{content}</InfoCard>
    </section>
  )
}

// validatePin returns the message for an unusable PIN pair, or null when pin
// is acceptable and matches confirmPin.
function validatePin(pin: string, confirmPin: string): string | null {
  if (pin.length < 4) return 'PIN must be at least 4 digits'
  if (pin !== confirmPin) return 'PINs do not match'
  return null
}

interface PinFieldsProps {
  pin: string
  confirmPin: string
  onPinChange: (pin: string) => void
  onConfirmPinChange: (confirmPin: string) => void
}

// PinFields renders the PIN and confirm PIN inputs, marking a mismatch.
function PinFields({
  sx,
  pin,
  confirmPin,
  onPinChange,
  onConfirmPinChange,
}: PinFieldsProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const pinId = useId()
  const confirmPinId = useId()

  return (
    <div sx={sx}>
      <div sx={nativeChildSpacing.s2}>
        <label sx={stylexStyles2.rule9} htmlFor={pinId}>
          PIN
        </label>
        <input
          id={pinId}
          type="password"
          value={pin}
          onChange={(e) => onPinChange(e.target.value)}
          placeholder="Enter PIN"
          sx={[stylexStyles2.rule10]}
        />
      </div>
      <div sx={nativeChildSpacing.s2}>
        <label sx={stylexStyles2.rule9} htmlFor={confirmPinId}>
          Confirm PIN
        </label>
        <input
          id={confirmPinId}
          type="password"
          value={confirmPin}
          onChange={(e) => onConfirmPinChange(e.target.value)}
          placeholder="Confirm PIN"
          sx={
            confirmPin.length > 0 && pin !== confirmPin
              ? stylexStyles2.rule11
              : stylexStyles2.rule10
          }
        />
      </div>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
