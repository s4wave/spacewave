import { LuFingerprint } from 'react-icons/lu'

import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { cn } from '@s4wave/web/style/utils.js'

// PasskeyChoice renders the existing-account and sign-up actions for a username.
export function PasskeyChoice({
  username,
  message,
  onExistingPasskey,
  onCreateAccount,
  onChangeUsername,
  onBack,
}: {
  username: string
  message: string
  onExistingPasskey: () => void
  onCreateAccount: () => void
  onChangeUsername: () => void
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
          <p className="text-foreground-alt max-w-sm text-sm">
            Choose how to continue for{' '}
            <span className="text-foreground font-medium">{username}</span>.
          </p>
        </>
      }
    >
      <div className="flex w-full flex-col gap-4">
        {message && (
          <div className="border-warning/20 bg-warning/10 rounded-md border px-3 py-2">
            <p className="text-foreground-alt text-xs">{message}</p>
          </div>
        )}
        <button
          onClick={onExistingPasskey}
          className={cn(
            'flex w-full items-center justify-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors',
            'bg-brand text-brand-foreground hover:bg-brand/90',
          )}
        >
          <LuFingerprint className="size-4" />
          Sign in with Passkey
        </button>
        <button
          onClick={onCreateAccount}
          className={cn(
            'border-foreground/20 text-foreground hover:border-foreground/30 hover:bg-background/40',
            'flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm font-medium transition-colors',
          )}
        >
          <LuFingerprint className="size-4" />
          Create New Passkey Account
        </button>
        <button
          onClick={onChangeUsername}
          className="text-brand hover:text-brand/80 text-sm underline"
        >
          Use a different username
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
