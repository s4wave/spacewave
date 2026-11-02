import { useCallback } from 'react'
import { LuArrowLeft, LuCheck, LuLock } from '@s4wave/web/images/icons.js'

import { cn } from '@s4wave/web/style/utils.js'

import {
  AuthCard,
  AuthPrimaryActionButton,
  AuthSecondaryActionButton,
  authInputClassName,
  getProviderLabel,
  ProviderIcon,
} from './auth-flow-shared.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-brand'],
  },
})

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
  const handlePinRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  return (
    <div className="flex flex-col gap-4">
      <AuthCard>
        <div className="mb-4 flex items-center gap-3">
          <div className="bg-brand/10 flex size-10 items-center justify-center rounded-lg">
            <ProviderIcon provider={provider} sx={stylexStyles2.rule1} />
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

        <div className="flex flex-col gap-4">
          <label className="flex flex-col gap-1.5">
            <span className="text-foreground-alt flex items-center gap-1.5 text-xs select-none">
              <LuLock sx={stylexStyles.rule1} />
              PIN
            </span>
            <input
              ref={handlePinRef}
              id="sso-unlock-pin"
              type="password"
              value={pin}
              onChange={(e) => onPinChange(e.target.value)}
              placeholder="Enter your PIN"
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
            {pinError ? (
              <p className="text-destructive text-xs">{pinError}</p>
            ) : (
              <p className="text-foreground-alt/50 text-xs">
                The PIN you set when this account was created.
              </p>
            )}
          </label>
          <AuthPrimaryActionButton
            onClick={onSubmit}
            disabled={busy || !pin}
            icon={<LuLock sx={stylexStyles.rule2} />}
          >
            Unlock and continue
          </AuthPrimaryActionButton>
          <AuthSecondaryActionButton
            onClick={onCancel}
            className="hover:text-brand flex items-center justify-center gap-1.5"
          >
            <LuArrowLeft sx={stylexStyles.rule3} />
            {cancelLabel}
          </AuthSecondaryActionButton>
        </div>
      </AuthCard>

      <div className="text-foreground-alt flex flex-wrap items-center justify-center gap-x-6 gap-y-1 text-xs">
        {UNLOCK_HIGHLIGHTS.map((text) => (
          <span key={text} className="flex items-center gap-1.5">
            <LuCheck sx={stylexStyles.rule4} />
            {text}
          </span>
        ))}
      </div>
    </div>
  )
}
