import type { ChangeEvent, RefCallback } from 'react'
import { LuArrowLeft, LuCheck, LuUserPlus } from 'react-icons/lu'

import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { cn } from '@s4wave/web/style/utils.js'

import {
  AuthCard,
  AuthPrimaryActionButton,
  AuthSecondaryActionButton,
  authInputClassName,
  getProviderLabel,
  ProviderIcon,
} from './auth-flow-shared.js'
import { OptionalPinLock } from './OptionalPinLock.js'

const SIGNUP_HIGHLIGHTS = ['End-to-end encrypted', 'Local-first', 'Open source']

// SSOConfirmForm collects a username and optional PIN for a new SSO account.
export function SSOConfirmForm({
  provider,
  email,
  username,
  usernameError,
  usernameInputRef,
  pin,
  confirmPin,
  pinError,
  confirmOpen,
  confirmUsername,
  tosHref,
  privacyHref,
  onUsernameChange,
  onPinChange,
  onConfirmPinChange,
  onRequestConfirm,
  onCancel,
  onCancelConfirm,
  onCreateAccount,
}: {
  provider: string
  email: string
  username: string
  usernameError: string
  usernameInputRef: RefCallback<HTMLInputElement>
  pin: string
  confirmPin: string
  pinError: string
  confirmOpen: boolean
  confirmUsername: string
  tosHref: string
  privacyHref: string
  onUsernameChange: (event: ChangeEvent<HTMLInputElement>) => void
  onPinChange: (value: string) => void
  onConfirmPinChange: (value: string) => void
  onRequestConfirm: () => void
  onCancel: () => void
  onCancelConfirm: () => void
  onCreateAccount: () => void
}) {
  const providerLabel = getProviderLabel(provider)

  return (
    <AuthScreenLayout
      alwaysShowIntro
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h2 className="text-foreground text-lg font-semibold">
            Welcome to Spacewave
          </h2>
          <p className="text-foreground-alt text-sm">
            Choose a username to finish signing up
          </p>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <AuthCard>
          <div className="mb-4 flex items-center gap-3">
            <div className="bg-brand/10 flex size-10 items-center justify-center rounded-lg">
              <ProviderIcon provider={provider} className="size-5" />
            </div>
            <div>
              <h2 className="text-foreground text-sm font-semibold">
                Sign up with {providerLabel}
              </h2>
              {email && <p className="text-foreground-alt text-xs">{email}</p>}
            </div>
          </div>

          <div className="flex flex-col gap-4">
            <label className="flex flex-col gap-1.5">
              <span className="text-foreground-alt text-xs select-none">
                Username
              </span>
              <input
                ref={usernameInputRef}
                value={username}
                onChange={onUsernameChange}
                placeholder="your-name"
                className={cn(
                  authInputClassName,
                  usernameError && 'border-destructive/50',
                )}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') onRequestConfirm()
                }}
              />
              {usernameError ? (
                <p className="text-destructive text-xs">{usernameError}</p>
              ) : (
                <p className="text-foreground-alt/50 text-xs">
                  Lowercase letters, numbers, and hyphens
                </p>
              )}
            </label>
            <OptionalPinLock
              pin={pin}
              confirmPin={confirmPin}
              pinError={pinError}
              onPinChange={onPinChange}
              onConfirmPinChange={onConfirmPinChange}
              onSubmit={onRequestConfirm}
              disabled={false}
              pinInputId="sso-pin"
            />
            <AuthPrimaryActionButton
              onClick={onRequestConfirm}
              disabled={!username || !!usernameError}
              icon={<LuUserPlus className="text-foreground size-4" />}
            >
              Create account
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={onCancel}
              className="hover:text-brand flex items-center justify-center gap-1.5"
            >
              <LuArrowLeft className="size-3" />
              Back to login
            </AuthSecondaryActionButton>
          </div>
        </AuthCard>

        <div className="text-foreground-alt flex flex-wrap items-center justify-center gap-x-6 gap-y-1 text-xs">
          {SIGNUP_HIGHLIGHTS.map((text) => (
            <span key={text} className="flex items-center gap-1.5">
              <LuCheck className="text-brand size-3.5" />
              {text}
            </span>
          ))}
        </div>
      </div>

      <Dialog
        open={confirmOpen}
        onOpenChange={(open) => {
          if (!open) onCancelConfirm()
        }}
      >
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>Confirm your username</DialogTitle>
            <DialogDescription>
              Your account will be created as{' '}
              <span className="text-foreground font-semibold">
                {confirmUsername}
              </span>
              . This username is permanent and cannot be changed later.
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-2">
            <AuthPrimaryActionButton
              onClick={onCreateAccount}
              icon={<LuUserPlus className="text-foreground size-4" />}
            >
              Confirm and create account
            </AuthPrimaryActionButton>
            <p className="text-foreground-alt/70 text-center text-xs">
              By clicking Confirm, you agree to our{' '}
              <a
                href={tosHref}
                target="_blank"
                rel="noopener noreferrer"
                className="text-brand hover:underline"
              >
                Terms of Service
              </a>{' '}
              and{' '}
              <a
                href={privacyHref}
                target="_blank"
                rel="noopener noreferrer"
                className="text-brand hover:underline"
              >
                Privacy Policy
              </a>
              .
            </p>
            <AuthSecondaryActionButton
              onClick={onCancelConfirm}
              className="hover:text-brand flex items-center justify-center gap-1.5"
            >
              <LuArrowLeft className="size-3" />
              Back to edit username
            </AuthSecondaryActionButton>
          </div>
        </DialogContent>
      </Dialog>
    </AuthScreenLayout>
  )
}
