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
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule2: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule3: { gap: `calc(${themeLayout['--spacing']} * 2)`, display: 'flex' },
  rule4: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule5: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule6: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule7: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule8: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule9: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    marginBottom: themeLayout['--spacing'],
  },
  rule10: {
    '--tw-outline-style': 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    outlineStyle: 'none',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule11: {
    '--tw-outline-style': 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 50%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    outlineStyle: 'none',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

export interface SessionLockSectionProps {
  sx?: stylex.StyleXStyles
  className?: never
  // embedded hides the section heading and outer wrapper when rendered inside
  // a parent CollapsibleSection.
  embedded?: boolean
}

// SessionLockSection displays session lock mode controls.
export function SessionLockSection({ sx, embedded }: SessionLockSectionProps) {
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

  const contentSx = getEmbeddedContentStyles(embedded, sx)
  const content = (
    <>
      {loading && (
        <LoadingInline
          sx={contentSx}
          label="Loading lock state"
          tone="muted"
          size="sm"
        />
      )}
      {!loading && (
        <div sx={contentSx}>
          <div sx={nativeChildSpacingNextNext.s3}>
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
              sx={nativeChildSpacingNextNext.s3}
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
              sx={[nativeChildSpacingNextNext.s3, stylexStyles2.rule1]}
            >
              Change PIN
            </button>
          )}

          {error && (
            <p sx={[nativeChildSpacingNextNext.s3, stylexStyles2.rule2]}>
              {error}
            </p>
          )}

          {hasChanges && (
            <div sx={[nativeChildSpacingNextNext.s3, stylexStyles2.rule3]}>
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

          <p sx={[nativeChildSpacingNextNext.s3, stylexStyles2.rule6]}>
            Changing lock mode only requires your current session. No account
            re-auth needed.
          </p>
        </div>
      )}
    </>
  )

  if (embedded) return content

  return (
    <section sx={sx}>
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
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNextNext = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})

// getEmbeddedContentStyles applies frame spacing only when there is no outer section.
function getEmbeddedContentStyles(
  embedded: boolean | undefined,
  sx: stylex.StyleXStyles,
): stylex.StyleXStyles {
  return embedded && sx
}
