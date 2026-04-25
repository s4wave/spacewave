import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  LuArrowLeft,
  LuBanknote,
  LuCircleCheck,
  LuLogOut,
  LuMail,
  LuRotateCcw,
  LuTimer,
  LuTriangleAlert,
} from 'react-icons/lu'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { SessionFrame } from '@s4wave/app/session/SessionFrame.js'
import {
  SessionContext,
  useSessionIndex,
} from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOnboardingContext } from '@s4wave/web/contexts/SpacewaveOnboardingContext.js'
import { useMountAccount } from '@s4wave/web/hooks/useMountAccount.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import { inputClass } from '@s4wave/web/ui/credential/CredentialProofInput.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { AccountLifecycleState } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

const cardClass = cn(
  'border-foreground/20 bg-background-get-started rounded-lg border p-5 shadow-lg backdrop-blur-sm',
)

function formatMoney(
  amount: bigint | number | null | undefined,
  currency: string,
) {
  if (amount == null) return null
  const cents = typeof amount === 'bigint' ? Number(amount) : amount
  return `$${(Math.abs(cents) / 100).toFixed(2)} ${currency.toUpperCase()}`
}

function formatDeleteSummary(result: {
  invoiceTotal?: bigint
  invoiceAmountDue?: bigint
  invoiceCurrency?: string
  invoiceStatus?: string
  chargeAttempted?: boolean
  refundAmount?: bigint
  refundCurrency?: string
}) {
  const currency = result.invoiceCurrency || result.refundCurrency || 'usd'
  if (result.refundAmount && result.refundAmount > 0n) {
    return (
      'Refund issued: ' +
      formatMoney(result.refundAmount, result.refundCurrency || currency)
    )
  }
  if (
    result.chargeAttempted &&
    result.invoiceAmountDue &&
    result.invoiceAmountDue > 0n
  ) {
    const amount = formatMoney(result.invoiceAmountDue, currency)
    if (result.invoiceStatus === 'paid') {
      return 'Final charge paid: ' + amount
    }
    return 'Final charge attempted; invoice remains open: ' + amount
  }
  if (result.invoiceTotal && result.invoiceTotal > 0n) {
    return (
      'Outstanding balance recorded: ' +
      formatMoney(result.invoiceTotal, currency)
    )
  }
  if (result.invoiceTotal && result.invoiceTotal < 0n) {
    return (
      'Final credit calculated: ' + formatMoney(result.invoiceTotal, currency)
    )
  }
  return 'Deletion confirmed. The account is now read-only.'
}

function formatCountdown(ms: number): { label: string; sub: string } {
  if (ms <= 0) return { label: 'Finalizing', sub: 'Deletion window has ended' }
  const totalSeconds = Math.floor(ms / 1000)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  const hh = String(hours).padStart(2, '0')
  const mm = String(minutes).padStart(2, '0')
  const ss = String(seconds).padStart(2, '0')
  return { label: `${hh}:${mm}:${ss}`, sub: 'remaining to undo' }
}

// DeleteCloudAccountPage drives the in-app delete-now email/code flow.
export function DeleteCloudAccountPage() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const onboarding = SpacewaveOnboardingContext.useContext().onboarding
  const navigate = useNavigate()
  const rootResource = useRootResource()
  const sessionIdx = useSessionIndex()
  const { providerId, accountId, peerId } = useSessionInfo(session)
  const accountResource = useMountAccount(providerId, accountId)
  const account = accountResource.value
  const lifecycleState = onboarding?.lifecycleState
  const deleteAt = onboarding?.deleteAt

  const [sending, setSending] = useState(false)
  const [verifying, setVerifying] = useState(false)
  const [undoing, setUndoing] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)
  const [email, setEmail] = useState('')
  const [retryAfter, setRetryAfter] = useState(0)
  const [code, setCode] = useState('')
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (retryAfter <= 0) return
    const id = setTimeout(() => setRetryAfter((v) => v - 1), 1000)
    return () => clearTimeout(id)
  }, [retryAfter])

  const isPendingDelete =
    lifecycleState ===
    AccountLifecycleState.AccountLifecycleState_PENDING_DELETE_READONLY

  useEffect(() => {
    if (!isPendingDelete || !deleteAt) return
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [isPendingDelete, deleteAt])

  const countdown = useMemo(() => {
    if (!isPendingDelete || !deleteAt) return null
    return formatCountdown(Number(deleteAt) - now)
  }, [isPendingDelete, deleteAt, now])

  const deleteAtLabel = useMemo(() => {
    if (!deleteAt) return null
    return new Date(Number(deleteAt)).toLocaleString()
  }, [deleteAt])

  const handleBack = useCallback(() => {
    navigate({ path: '../' })
  }, [navigate])

  const handleSend = useCallback(async () => {
    if (!session) return
    setSending(true)
    try {
      const resp = await session.spacewave.requestDeleteNowEmail()
      setEmail(resp.email ?? '')
      setRetryAfter(resp.retryAfter ?? 0)
      toast.success('Confirmation email sent')
    } catch (err) {
      toast.error(
        err instanceof Error ?
          err.message
        : 'Failed to send confirmation email',
      )
    } finally {
      setSending(false)
    }
  }, [session])

  const handleConfirm = useCallback(async () => {
    if (!session || code.length !== 6) return
    setVerifying(true)
    try {
      const resp = await session.spacewave.confirmDeleteNowCode(code)
      toast.success(formatDeleteSummary(resp))
      navigate({ path: '../', replace: true })
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Delete confirmation failed',
      )
    } finally {
      setVerifying(false)
    }
  }, [code, navigate, session])

  const handleUndo = useCallback(async () => {
    if (!session) return
    setUndoing(true)
    try {
      await session.spacewave.undoDeleteNow()
      toast.success(
        'Deletion canceled. The account stays read-only until you resubscribe.',
      )
      navigate({ path: '../', replace: true })
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Failed to cancel deletion',
      )
    } finally {
      setUndoing(false)
    }
  }, [navigate, session])

  const handleLogout = useCallback(async () => {
    if (!account || !peerId || peerId === 'Unknown') return
    setLoggingOut(true)
    await account.selfRevokeSession(peerId).catch(() => {})
    const root = rootResource.value
    if (root && sessionIdx != null) {
      await root.deleteSession(sessionIdx).catch(() => {})
    }
    navigate({ path: '/sessions', replace: true })
  }, [account, navigate, peerId, rootResource.value, sessionIdx])

  return (
    <SessionFrame>
      <div className="bg-background-landing relative flex flex-1 flex-col items-center overflow-y-auto p-6 outline-none md:p-10">
        <ShootingStars className="pointer-events-none fixed inset-0 opacity-60" />
        <div className="relative z-10 my-auto flex w-full max-w-lg flex-col gap-4">
          <button
            onClick={handleBack}
            className="text-foreground-alt hover:text-foreground -mb-2 flex items-center gap-1.5 self-start text-xs transition-colors select-none"
          >
            <LuArrowLeft className="h-3.5 w-3.5" />
            Back
          </button>

          <div className="flex flex-col items-center gap-2">
            <AnimatedLogo followMouse={false} />
            <h1 className="mt-2 text-xl font-bold tracking-wide select-none">
              {isPendingDelete ? 'Deletion Scheduled' : 'Delete Account'}
            </h1>
            <p className="text-foreground-alt max-w-sm text-center text-sm">
              {isPendingDelete ?
                'Your cloud account is read-only. You can still undo before the window closes.'
              : "Close your Spacewave Cloud account. You'll have 24 hours to undo it before deletion is final."
              }
            </p>
          </div>

          {isPendingDelete ?
            <PendingDeleteView
              countdown={countdown}
              deleteAtLabel={deleteAtLabel}
              undoing={undoing}
              loggingOut={loggingOut}
              onUndo={() => void handleUndo()}
              onDashboard={handleBack}
              onLogout={() => void handleLogout()}
            />
          : <InitiateDeleteView
              sending={sending}
              verifying={verifying}
              retryAfter={retryAfter}
              email={email}
              code={code}
              onCodeChange={setCode}
              onSend={() => void handleSend()}
              onConfirm={() => void handleConfirm()}
              onCancel={handleBack}
            />
          }
        </div>
      </div>
    </SessionFrame>
  )
}

// InitiateDeleteView renders the card stack used before deletion starts.
function InitiateDeleteView({
  sending,
  verifying,
  retryAfter,
  email,
  code,
  onCodeChange,
  onSend,
  onConfirm,
  onCancel,
}: {
  sending: boolean
  verifying: boolean
  retryAfter: number
  email: string
  code: string
  onCodeChange: (v: string) => void
  onSend: () => void
  onConfirm: () => void
  onCancel: () => void
}) {
  const codeValid = code.length === 6
  const canSend = !sending && retryAfter <= 0

  return (
    <>
      <div className={cardClass}>
        <h2 className="text-foreground mb-3 text-xs font-medium tracking-wide uppercase select-none">
          What happens
        </h2>
        <ul className="flex flex-col gap-3">
          <InfoRow
            icon={<LuTimer className="text-destructive h-4 w-4" />}
            title="Subscription ends now"
            body="Your account becomes read-only. We'll issue the final invoice immediately."
          />
          <InfoRow
            icon={<LuRotateCcw className="text-brand h-4 w-4" />}
            title="24 hours to undo"
            body="Before the window closes, you can cancel deletion from this page or the confirmation email."
          />
          <InfoRow
            icon={<LuBanknote className="text-foreground-alt h-4 w-4" />}
            title="Prorated refund if any"
            body="Any unused time is refunded to your card, less Stripe's $0.30 processing fee."
          />
        </ul>
      </div>

      <div className={cn(cardClass, 'space-y-4')}>
        <div className="flex items-start gap-3">
          <div className="bg-destructive/10 flex h-10 w-10 shrink-0 items-center justify-center rounded-lg">
            <LuTriangleAlert className="text-destructive h-5 w-5" />
          </div>
          <div className="flex-1">
            <h2 className="text-foreground text-sm font-semibold select-none">
              Confirm by email
            </h2>
            <p className="text-foreground-alt text-xs leading-relaxed">
              {email ?
                <>
                  We sent a link and 6-digit code to{' '}
                  <strong className="text-foreground">{email}</strong>. Click
                  the link or enter the code below.
                </>
              : "We'll email you a confirmation link and a 6-digit code. Use either to finalize deletion."
              }
            </p>
          </div>
        </div>

        <button
          onClick={onSend}
          disabled={!canSend}
          className={cn(
            'flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm transition-colors',
            'border-brand/30 bg-brand/10 hover:bg-brand/20',
            'disabled:cursor-not-allowed disabled:opacity-50',
          )}
        >
          {sending ?
            <Spinner />
          : email ?
            <LuCircleCheck className="text-brand h-4 w-4" />
          : <LuMail className="h-4 w-4" />}
          <span className="text-foreground">
            {sending ?
              'Sending...'
            : retryAfter > 0 ?
              `Resend in ${retryAfter}s`
            : email ?
              'Resend confirmation email'
            : 'Send confirmation email'}
          </span>
        </button>

        <div className="space-y-1.5">
          <label className="text-foreground-alt block text-xs select-none">
            6-digit delete code
          </label>
          <input
            type="text"
            inputMode="numeric"
            maxLength={6}
            value={code}
            onChange={(e) => onCodeChange(e.target.value.replace(/\D/g, ''))}
            placeholder="000000"
            className={cn(
              inputClass,
              'text-center font-mono text-lg tracking-[0.3em]',
            )}
          />
        </div>

        <div className="flex flex-col gap-2 pt-1">
          <button
            onClick={onConfirm}
            disabled={verifying || !codeValid}
            className={cn(
              'flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm font-medium transition-colors',
              'border-destructive/30 bg-destructive/10 text-destructive hover:bg-destructive/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            {verifying && <Spinner />}
            {verifying ? 'Confirming...' : 'Confirm delete account'}
          </button>
          <button
            onClick={onCancel}
            className="text-foreground-alt hover:text-foreground text-center text-xs transition-colors select-none"
          >
            Cancel and return to dashboard
          </button>
        </div>
      </div>
    </>
  )
}

// PendingDeleteView renders the undo-focused state after deletion is scheduled.
function PendingDeleteView({
  countdown,
  deleteAtLabel,
  undoing,
  loggingOut,
  onUndo,
  onDashboard,
  onLogout,
}: {
  countdown: { label: string; sub: string } | null
  deleteAtLabel: string | null
  undoing: boolean
  loggingOut: boolean
  onUndo: () => void
  onDashboard: () => void
  onLogout: () => void
}) {
  return (
    <>
      {countdown && (
        <div
          className={cn(
            cardClass,
            'border-warning/30 bg-warning/5 flex flex-col items-center gap-1 py-6',
          )}
        >
          <span className="text-foreground-alt text-xs tracking-wider uppercase select-none">
            Undo window
          </span>
          <span className="text-foreground font-mono text-4xl font-semibold tabular-nums">
            {countdown.label}
          </span>
          <span className="text-foreground-alt text-xs">{countdown.sub}</span>
          {deleteAtLabel && (
            <span className="text-foreground-alt/70 mt-1 text-[0.65rem]">
              Deletion final at {deleteAtLabel}
            </span>
          )}
        </div>
      )}

      <div className={cn(cardClass, 'space-y-3')}>
        <button
          onClick={onUndo}
          disabled={undoing}
          className={cn(
            'flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2.5 text-sm font-medium transition-colors',
            'border-brand/40 bg-brand/15 text-foreground hover:bg-brand/25',
            'disabled:cursor-not-allowed disabled:opacity-50',
          )}
        >
          {undoing ?
            <Spinner />
          : <LuRotateCcw className="h-4 w-4" />}
          {undoing ? 'Canceling...' : 'Undo deletion'}
        </button>
        <p className="text-foreground-alt text-center text-xs leading-relaxed">
          Undo stops deletion, but the account stays lapsed and read-only until
          you start a new subscription.
        </p>
      </div>

      <div className="flex flex-col gap-2">
        <button
          onClick={onDashboard}
          className="text-foreground-alt hover:text-foreground flex items-center justify-center gap-1.5 text-xs transition-colors select-none"
        >
          <LuArrowLeft className="h-3 w-3" />
          Return to dashboard
        </button>
        <button
          onClick={onLogout}
          disabled={loggingOut}
          className="text-foreground-alt hover:text-foreground flex items-center justify-center gap-1.5 text-xs transition-colors select-none disabled:cursor-not-allowed disabled:opacity-50"
        >
          <LuLogOut className="h-3 w-3" />
          {loggingOut ? 'Logging out...' : 'Log out of this device'}
        </button>
      </div>
    </>
  )
}

// InfoRow renders a single icon + title + body bullet used in the info card.
function InfoRow({
  icon,
  title,
  body,
}: {
  icon: React.ReactNode
  title: string
  body: string
}) {
  return (
    <li className="flex items-start gap-3">
      <div className="bg-foreground/5 mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md">
        {icon}
      </div>
      <div className="flex-1">
        <p className="text-foreground text-sm font-medium select-none">
          {title}
        </p>
        <p className="text-foreground-alt text-xs leading-relaxed">{body}</p>
      </div>
    </li>
  )
}
