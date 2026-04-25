import { LuLock } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

import { authInputClassName } from './auth-flow-shared.js'

export interface OptionalPinLockProps {
  pin: string
  confirmPin: string
  pinError: string
  onPinChange: (value: string) => void
  onConfirmPinChange: (value: string) => void
  onSubmit: () => void
  disabled: boolean
  pinInputId: string
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
  } = props

  return (
    <div className="flex w-full flex-col gap-2">
      <div className="flex items-center gap-2">
        <LuLock className="text-foreground-alt h-4 w-4" />
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
          if (e.key === 'Enter' && !disabled) {
            onSubmit()
          }
        }}
      />
      <input
        type="password"
        value={confirmPin}
        onChange={(e) => onConfirmPinChange(e.target.value)}
        placeholder="Confirm PIN"
        disabled={disabled}
        className={cn(authInputClassName, pinError && 'border-destructive')}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && !disabled) {
            onSubmit()
          }
        }}
      />
      {pinError && <p className="text-destructive text-xs">{pinError}</p>}
    </div>
  )
}
