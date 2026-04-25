import { LuArrowLeft, LuCheck, LuLock } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

import {
  AuthCard,
  AuthPrimaryActionButton,
  AuthSecondaryActionButton,
  authInputClassName,
  getProviderLabel,
  ProviderIcon,
} from './auth-flow-shared.js'

export interface SSOUnlockCardProps {
  provider: string
  email?: string
  username?: string
  pin: string
  pinError: string
  busy: boolean
  onPinChange: (value: string) => void
  onSubmit: () => void
  onCancel: () => void
  cancelLabel?: string
}

const UNLOCK_HIGHLIGHTS = ['End-to-end encrypted', 'Local-first', 'Open source']

// SSOUnlockCard renders the post-SSO PIN unlock card. Mirrors SSOConfirmPage's
// provider context header, AuthCard wrapper, and trust-signal row so the
// signed-in identity flow stays visually continuous with the signup flow.
export function SSOUnlockCard(props: SSOUnlockCardProps) {
  const {
    provider,
    email,
    username,
    pin,
    pinError,
    busy,
    onPinChange,
    onSubmit,
    onCancel,
    cancelLabel = 'Back to login',
  } = props
  const providerLabel = getProviderLabel(provider)

  return (
    <div className="flex flex-col gap-4">
      <AuthCard>
        <div className="mb-4 flex items-center gap-3">
          <div className="bg-brand/10 flex h-10 w-10 items-center justify-center rounded-lg">
            <ProviderIcon provider={provider} className="h-5 w-5" />
          </div>
          <div className="flex min-w-0 flex-col">
            <h2 className="text-foreground text-sm font-semibold">
              Signing in with {providerLabel}
            </h2>
            {username && (
              <p className="text-foreground truncate text-xs font-medium">
                {username}
              </p>
            )}
            {email && (
              <p className="text-foreground-alt truncate text-xs">{email}</p>
            )}
          </div>
        </div>

        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (!busy) onSubmit()
          }}
        >
          <label className="flex flex-col gap-1.5">
            <span className="text-foreground-alt flex items-center gap-1.5 text-xs select-none">
              <LuLock className="h-3.5 w-3.5" />
              PIN
            </span>
            <input
              id="sso-unlock-pin"
              type="password"
              value={pin}
              onChange={(e) => onPinChange(e.target.value)}
              placeholder="Enter your PIN"
              autoFocus
              disabled={busy}
              className={cn(
                authInputClassName,
                pinError && 'border-destructive/50',
              )}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !busy) {
                  e.preventDefault()
                  onSubmit()
                }
              }}
            />
            {pinError ?
              <p className="text-destructive text-xs">{pinError}</p>
            : <p className="text-foreground-alt/50 text-xs">
                The PIN you set when this account was created.
              </p>
            }
          </label>
          <AuthPrimaryActionButton
            type="submit"
            disabled={busy || !pin}
            icon={<LuLock className="text-foreground h-4 w-4" />}
          >
            Unlock and continue
          </AuthPrimaryActionButton>
          <AuthSecondaryActionButton
            onClick={onCancel}
            className="hover:text-brand flex items-center justify-center gap-1.5"
          >
            <LuArrowLeft className="h-3 w-3" />
            {cancelLabel}
          </AuthSecondaryActionButton>
        </form>
      </AuthCard>

      <div className="text-foreground-alt flex flex-wrap items-center justify-center gap-x-6 gap-y-1 text-xs">
        {UNLOCK_HIGHLIGHTS.map((text) => (
          <span key={text} className="flex items-center gap-1.5">
            <LuCheck className="text-brand h-3.5 w-3.5" />
            {text}
          </span>
        ))}
      </div>
    </div>
  )
}
