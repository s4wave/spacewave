import type { ChangeEvent, RefCallback } from 'react'
import { LuFingerprint } from 'react-icons/lu'

import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { cn } from '@s4wave/web/style/utils.js'

import { authInputClassName } from './auth-flow-shared.js'

// PasskeyUsernameForm collects the account name before selecting a passkey flow.
export function PasskeyUsernameForm({
  username,
  usernameError,
  usernameInputRef,
  onUsernameChange,
  onContinue,
  onBack,
}: {
  username: string
  usernameError: string
  usernameInputRef: RefCallback<HTMLInputElement>
  onUsernameChange: (event: ChangeEvent<HTMLInputElement>) => void
  onContinue: () => void
  onBack: () => void
}) {
  return (
    <AuthScreenLayout
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h2 className="text-foreground text-lg font-semibold">
            Continue with passkey
          </h2>
        </>
      }
    >
      <div className="flex w-full flex-col gap-6">
        <div className="flex w-full flex-col gap-2">
          <label
            htmlFor="passkey-username"
            className="text-foreground text-sm font-medium"
          >
            Username
          </label>
          <input
            ref={usernameInputRef}
            id="passkey-username"
            type="text"
            value={username}
            onChange={onUsernameChange}
            placeholder="your-username"
            autoComplete="username webauthn"
            className={cn(
              authInputClassName,
              usernameError && 'border-destructive',
            )}
            onKeyDown={(event) => {
              if (event.key === 'Enter') onContinue()
            }}
          />
          {usernameError && (
            <p className="text-destructive text-xs">{usernameError}</p>
          )}
        </div>

        <button
          onClick={onContinue}
          disabled={!username || !!usernameError}
          className={cn(
            'flex w-full items-center justify-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors',
            'bg-brand text-brand-foreground hover:bg-brand/90',
            'disabled:cursor-not-allowed disabled:opacity-50',
          )}
        >
          <LuFingerprint className="size-4" />
          Continue
        </button>

        <button
          onClick={onBack}
          className="text-foreground-alt hover:text-foreground text-xs transition-colors"
        >
          Back to login
        </button>
      </div>
    </AuthScreenLayout>
  )
}
