import type { MouseEventHandler, ReactNode } from 'react'
import { FcGoogle } from 'react-icons/fc'
import { LuGithub } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { SpacewaveProvider } from '@s4wave/sdk/provider/spacewave/spacewave.js'
import type { Root } from '@s4wave/sdk/root/root.js'

import { dnsLabelRegex } from './keypair-utils.js'

export const authInputClassName = cn(
  'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50',
  'w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none',
  'focus:border-brand/50',
)

export function getProviderLabel(provider: string): string {
  if (provider === 'google') return 'Google'
  if (provider === 'github') return 'GitHub'
  return provider
}

export function ProviderIcon({
  provider,
  className,
}: {
  provider: string
  className?: string
}) {
  if (provider === 'google') return <FcGoogle className={className} />
  if (provider === 'github') return <LuGithub className={className} />
  return null
}

export function normalizeUsernameInput(value: string): {
  username: string
  error: string
} {
  const username = value.toLowerCase()
  if (username && !dnsLabelRegex.test(username)) {
    return {
      username,
      error: 'Lowercase letters, numbers, and hyphens only',
    }
  }
  return { username, error: '' }
}

export function validateUsername(username: string): string | null {
  if (!username || !dnsLabelRegex.test(username)) {
    return 'Please enter a valid username'
  }
  return null
}

export function validateOptionalPin(
  pin: string,
  confirmPin: string,
): string | null {
  const wantsPin = pin.length > 0 || confirmPin.length > 0
  if (!wantsPin) return null
  if (!pin || !confirmPin) {
    return 'Enter and confirm a PIN, or leave both fields blank'
  }
  if (pin !== confirmPin) {
    return 'PINs do not match'
  }
  return null
}

export function isUsernameTakenError(err: unknown): boolean {
  const message = err instanceof Error ? err.message : String(err)
  return (
    message.includes('username_taken') ||
    message.includes('Username already exists')
  )
}

export function getErrorMessage(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback
}

export async function withSpacewaveProvider<T>(
  root: Root,
  fn: (spacewave: SpacewaveProvider) => Promise<T>,
  abortSignal?: AbortSignal,
): Promise<T> {
  using provider = await root.lookupProvider('spacewave', abortSignal)
  return await fn(new SpacewaveProvider(provider.resourceRef))
}

export async function loginWithEntityPem(
  root: Root,
  pemPrivateKey: Uint8Array,
): Promise<number> {
  return await withSpacewaveProvider(root, async (spacewave) => {
    const loginResp = await spacewave.loginWithEntityKey(pemPrivateKey)
    return loginResp.sessionListEntry?.sessionIndex ?? 0
  })
}

export function AuthCard({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        'border-foreground/20 bg-background-get-started rounded-lg border p-5 shadow-lg backdrop-blur-sm',
        className,
      )}
    >
      {children}
    </div>
  )
}

export function AuthStatusPanel({
  icon,
  message,
  children,
}: {
  icon: ReactNode
  message: ReactNode
  children?: ReactNode
}) {
  return (
    <AuthCard>
      <div className="flex flex-col items-center gap-4 text-center">
        {icon}
        <p className="text-foreground-alt text-sm">{message}</p>
        {children}
      </div>
    </AuthCard>
  )
}

export function AuthPrimaryActionButton({
  onClick,
  icon,
  children,
  disabled,
  type = 'button',
}: {
  onClick?: MouseEventHandler<HTMLButtonElement>
  icon?: ReactNode
  children: ReactNode
  disabled?: boolean
  type?: 'button' | 'submit'
}) {
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      className={cn(
        'w-full rounded-md border transition-all duration-300',
        'border-brand/30 bg-brand/10 hover:bg-brand/20',
        'disabled:cursor-not-allowed disabled:opacity-50',
        'flex h-10 items-center justify-center gap-2',
      )}
    >
      {icon}
      <span className="text-foreground text-sm">{children}</span>
    </button>
  )
}

export function AuthSecondaryActionButton({
  onClick,
  children,
  className,
}: {
  onClick?: MouseEventHandler<HTMLButtonElement>
  children: ReactNode
  className?: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'text-foreground-alt hover:text-foreground text-sm transition-colors',
        className,
      )}
    >
      {children}
    </button>
  )
}
