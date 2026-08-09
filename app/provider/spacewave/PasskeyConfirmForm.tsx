import type { ChangeEvent, RefCallback } from 'react'
import { LuArrowLeft, LuFingerprint, LuUserPlus } from 'react-icons/lu'

import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'

import {
  AuthCard,
  AuthPrimaryActionButton,
  AuthSecondaryActionButton,
  authInputClassName,
} from './auth-flow-shared.js'
import { OptionalPinLock } from './OptionalPinLock.js'

const HIGHLIGHTS = ['End-to-end encrypted', 'Passkey-protected', 'Local-first']

// PasskeyConfirmForm collects the account details for a desktop passkey sign-up.
export function PasskeyConfirmForm({
  username,
  placeholderUsername,
  usernameError,
  usernameInputRef,
  pin,
  confirmPin,
  pinError,
  isBusy,
  statusMessage,
  onUsernameChange,
  onPinChange,
  onConfirmPinChange,
  onCreateAccount,
  onCancel,
}: {
  username: string
  placeholderUsername: string
  usernameError: string
  usernameInputRef: RefCallback<HTMLInputElement>
  pin: string
  confirmPin: string
  pinError: string
  isBusy: boolean
  statusMessage: string
  onUsernameChange: (event: ChangeEvent<HTMLInputElement>) => void
  onPinChange: (value: string) => void
  onConfirmPinChange: (value: string) => void
  onCreateAccount: () => void
  onCancel: () => void
}) {
  return (
    <AuthScreenLayout
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <div className="flex items-center gap-2">
            <div className="bg-brand/10 border-brand/30 flex size-11 items-center justify-center rounded-full border">
              <LuFingerprint className="text-brand size-5" />
            </div>
            <div className="text-left">
              <h2 className="text-foreground text-lg font-semibold">
                Finish passkey setup
              </h2>
              <p className="text-foreground-alt text-sm">
                Choose your Spacewave username to finish desktop sign-in.
              </p>
            </div>
          </div>
        </>
      }
    >
      <div className="flex w-full flex-col gap-6">
        <div className="grid gap-2 sm:grid-cols-3">
          {HIGHLIGHTS.map((item) => (
            <div
              key={item}
              className="border-foreground/10 bg-background/20 text-foreground-alt rounded-md border px-3 py-2 text-xs"
            >
              {item}
            </div>
          ))}
        </div>

        <AuthCard className="flex flex-col gap-6">
          <div className="flex w-full flex-col gap-2">
            <label
              htmlFor="desktop-passkey-username"
              className="text-foreground text-sm font-medium"
            >
              Username
            </label>
            <input
              ref={usernameInputRef}
              id="desktop-passkey-username"
              type="text"
              value={username}
              onChange={onUsernameChange}
              placeholder={placeholderUsername || 'your-username'}
              disabled={isBusy}
              className={cn(
                authInputClassName,
                usernameError && 'border-destructive',
              )}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !isBusy) onCreateAccount()
              }}
            />
            {usernameError && (
              <p className="text-destructive text-xs">{usernameError}</p>
            )}
          </div>

          <OptionalPinLock
            pin={pin}
            confirmPin={confirmPin}
            pinError={pinError}
            onPinChange={onPinChange}
            onConfirmPinChange={onConfirmPinChange}
            onSubmit={onCreateAccount}
            disabled={isBusy}
            pinInputId="desktop-passkey-pin"
          />

          <div className="flex w-full flex-col gap-2">
            <AuthPrimaryActionButton
              onClick={onCreateAccount}
              disabled={isBusy || !username || !!usernameError}
              icon={isBusy ? <Spinner /> : <LuUserPlus className="size-4" />}
            >
              {isBusy ? statusMessage : 'Create account'}
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={onCancel}
              className="flex items-center justify-center gap-2"
            >
              <LuArrowLeft className="size-4" />
              Back to login
            </AuthSecondaryActionButton>
          </div>
        </AuthCard>
      </div>
    </AuthScreenLayout>
  )
}
