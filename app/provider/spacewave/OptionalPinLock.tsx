import { LuLock } from '@s4wave/web/images/icons.js'

import { authInputStyles } from './auth-flow-shared.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import { fonts as themeFonts } from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: 'column',
    width: '100%',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule4: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

export interface OptionalPinLockProps {
  pin: string
  confirmPin: string
  pinError: string
  onPinChange: (value: string) => void
  onConfirmPinChange: (value: string) => void
  onSubmit: () => void
  disabled: boolean
  pinInputId: string
  // keyHeldWithoutPin is set when skipping the PIN leaves the entity key in
  // Spacewave Cloud custody, readable by the server without any user secret.
  keyHeldWithoutPin?: boolean
}

// OptionalPinLock renders the optional PIN + confirm-PIN credential entry
// shared by passkey and SSO signup confirmation pages.
export function OptionalPinLock(props: OptionalPinLockProps) {
  const {
    pin,
    confirmPin,
    pinError,
    onPinChange,
    onConfirmPinChange,
    onSubmit,
    disabled,
    pinInputId,
    keyHeldWithoutPin,
  } = props

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <LuLock sx={stylexStyles.rule1} />
        <label htmlFor={pinInputId} sx={stylexStyles2.rule3}>
          Optional PIN lock
        </label>
      </div>
      <input
        id={pinInputId}
        type="password"
        value={pin}
        onChange={(e) => onPinChange(e.target.value)}
        placeholder="Leave blank to skip"
        disabled={disabled}
        sx={authInputStyles.base}
        onKeyDown={(e) => {
          if (e.nativeEvent.isComposing) return
          if (e.key === 'Enter' && !disabled) {
            onSubmit()
          }
        }}
      />
      <input
        aria-label="Confirm PIN"
        type="password"
        value={confirmPin}
        onChange={(e) => onConfirmPinChange(e.target.value)}
        placeholder="Confirm PIN"
        disabled={disabled}
        sx={pinError ? authInputStyles.invalid : authInputStyles.base}
        onKeyDown={(e) => {
          if (e.nativeEvent.isComposing) return
          if (e.key === 'Enter' && !disabled) {
            onSubmit()
          }
        }}
      />
      {pinError && <p sx={stylexStyles2.rule4}>{pinError}</p>}
      {keyHeldWithoutPin && !pin && (
        <p sx={stylexStyles2.rule5}>
          Without a PIN, Spacewave Cloud keeps your account key so you can sign
          in on any device, and we could use it to open your data. With a long
          PIN, only you can unlock the key.
        </p>
      )}
    </div>
  )
}
