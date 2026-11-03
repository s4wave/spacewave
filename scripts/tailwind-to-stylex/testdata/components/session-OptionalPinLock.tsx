import { LuLock } from '@s4wave/web/images/icons.js'

import { cn } from '@s4wave/web/style/utils.js'

import { authInputClassName } from './auth-flow-shared.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
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
    <div className="flex w-full flex-col gap-2">
      <div className="flex items-center gap-2">
        <LuLock sx={stylexStyles.rule1} />
        <label
          htmlFor={pinInputId}
          className="text-foreground text-sm font-medium"
        >
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
        className={authInputClassName}
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
        className={cn(authInputClassName, pinError && 'border-destructive')}
        onKeyDown={(e) => {
          if (e.nativeEvent.isComposing) return
          if (e.key === 'Enter' && !disabled) {
            onSubmit()
          }
        }}
      />
      {pinError && <p className="text-destructive text-xs">{pinError}</p>}
      {keyHeldWithoutPin && !pin && (
        <p className="text-foreground-alt text-xs">
          Without a PIN, Spacewave Cloud keeps your account key so you can sign
          in on any device, and we could use it to open your data. With a long
          PIN, only you can unlock the key.
        </p>
      )}
    </div>
  )
}
