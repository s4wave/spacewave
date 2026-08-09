import { LuCircleAlert } from 'react-icons/lu'

// PasskeyError renders passkey-flow failures with retry and exit actions.
export function PasskeyError({
  message,
  onRetry,
  onBack,
}: {
  message: string
  onRetry: () => void
  onBack: () => void
}) {
  return (
    <div className="bg-background-landing relative flex flex-1 flex-col items-center justify-center p-6">
      <div className="relative z-10 flex flex-col items-center gap-4 text-center">
        <LuCircleAlert className="text-destructive size-12" />
        <h2 className="text-foreground text-lg font-semibold">
          Passkey sign-in failed
        </h2>
        <p className="text-foreground-alt max-w-sm text-sm">{message}</p>
        <button
          onClick={onRetry}
          className="text-brand hover:text-brand/80 mt-2 text-sm underline"
        >
          Try again
        </button>
        <button
          onClick={onBack}
          className="text-foreground-alt hover:text-foreground text-xs transition-colors"
        >
          Back to login
        </button>
      </div>
    </div>
  )
}
