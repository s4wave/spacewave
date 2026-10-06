import { useCallback, useState, type ReactNode } from 'react'
import {
  LuArrowRight,
  LuCheck,
  LuChevronDown,
  LuMail,
  LuPlus,
  LuSend,
  LuTrash2,
} from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'
import { inputClass } from '@s4wave/web/ui/credential/CredentialProofInput.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { SessionFrame } from '@s4wave/app/session/SessionFrame.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { useEmailManagement } from '@s4wave/web/hooks/useEmailManagement.js'
import { SpacewaveOnboardingContext } from '@s4wave/web/contexts/SpacewaveOnboardingContext.js'
import type { EmailInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

interface EmailVisibility {
  hasVerified: boolean
  hasUnverified: boolean
  visibleEmails: EmailInfo[]
}

/**
 * emailVisibility decides which emails the gate shows. Onboarding Status
 * decides account-level verification; the email list may still be one fetch
 * behind that gate, so either source marks the account verified.
 */
function emailVisibility(
  emails: EmailInfo[] | null | undefined,
  accountEmailVerified: boolean,
): EmailVisibility {
  const hasVerified =
    accountEmailVerified || (emails?.some((e) => e.verified) ?? false)
  return {
    hasVerified,
    hasUnverified: emails?.some((e) => !e.verified) ?? false,
    visibleEmails: hasVerified
      ? (emails?.filter((e) => e.verified) ?? [])
      : (emails ?? []),
  }
}

/** ContinueButton leaves the gate once an email is verified. */
function ContinueButton() {
  const navigate = useNavigate()

  return (
    <button
      type="button"
      onClick={() => navigate({ path: '../' })}
      className={cn(
        'flex w-full cursor-pointer items-center justify-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300 select-none',
        'border-brand bg-brand/10 text-foreground hover:bg-brand/20',
      )}
    >
      Continue
      <LuArrowRight className="size-4" />
    </button>
  )
}

/** AddEmailCard is the collapsible form for verifying a different address. */
function AddEmailCard({
  busy,
  onAddEmail,
}: {
  busy: boolean
  onAddEmail: (email: string) => Promise<boolean>
}) {
  const [expanded, setExpanded] = useState(false)
  const [newEmail, setNewEmail] = useState('')
  const handleInputRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  const handleAdd = useCallback(async () => {
    if (!newEmail) return
    const ok = await onAddEmail(newEmail)
    if (!ok) return
    setNewEmail('')
    setExpanded(false)
  }, [onAddEmail, newEmail])

  return (
    <div className="border-foreground/20 bg-background-get-started overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
      <button
        type="button"
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center gap-3 p-4"
      >
        <div className="bg-foreground/5 flex size-8 shrink-0 items-center justify-center rounded-lg">
          <LuPlus className="text-foreground-alt size-4" />
        </div>
        <div className="flex-1 text-left">
          <h3 className="text-foreground text-sm font-medium">
            Use a different email
          </h3>
          <p className="text-foreground-alt text-xs">
            Add another address to verify instead
          </p>
        </div>
        <LuChevronDown
          className={cn(
            'text-foreground-alt size-4 shrink-0 transition-transform duration-200',
            expanded && 'rotate-180',
          )}
        />
      </button>
      {expanded && (
        <div className="border-foreground/10 space-y-3 border-t px-4 pt-3 pb-4">
          <input
            aria-label="New email address"
            ref={handleInputRef}
            type="email"
            placeholder="you@example.com"
            value={newEmail}
            onChange={(e) => setNewEmail(e.target.value)}
            onKeyDown={(e) => {
              if (e.nativeEvent.isComposing) return
              if (e.key === 'Enter') {
                void handleAdd()
              }
            }}
            className={inputClass}
          />
          <button
            type="button"
            onClick={() => void handleAdd()}
            disabled={busy || !newEmail}
            className={cn(
              'group w-full rounded-md border transition-all duration-300',
              'border-brand/30 bg-brand/10 hover:bg-brand/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
              'flex h-10 items-center justify-center gap-2',
            )}
          >
            <LuSend className="text-foreground size-4" />
            <span className="text-foreground text-sm">
              {busy ? 'Adding…' : 'Add & send code'}
            </span>
          </button>
        </div>
      )}
    </div>
  )
}

// VerifyEmailPage renders the email verification gate page.
// Shown after checkout when the user has no verified email.
export function VerifyEmailPage() {
  const onboarding = SpacewaveOnboardingContext.useContextSafe()
  const {
    emails,
    loading,
    verifyingEmail,
    code,
    setCode,
    retryAfter,
    sendingCode,
    verifyingCode,
    addingEmail,
    removingEmail,
    sendCode,
    verifyCode,
    addEmail,
    removeEmail,
  } = useEmailManagement()

  const busy = verifyingCode || addingEmail || removingEmail !== null
  const { hasVerified, hasUnverified, visibleEmails } = emailVisibility(
    emails,
    onboarding?.emailVerified ?? false,
  )
  const showSendHint =
    !hasVerified && hasUnverified && !verifyingEmail && !sendingCode

  return (
    <SessionFrame>
      <div className="bg-background-landing relative flex flex-1 flex-col items-center overflow-y-auto p-6 outline-none md:p-10">
        <div className="relative z-10 my-auto flex w-full max-w-lg flex-col gap-4">
          {/* Header */}
          <div className="flex flex-col items-center gap-2">
            <AnimatedLogo followMouse={false} />
            <h1 className="mt-2 text-xl font-semibold tracking-wide">
              Verify Your Email
            </h1>
            <p className="text-foreground-alt text-center text-sm">
              Confirm your email address to start using Spacewave Cloud.
            </p>
          </div>

          {/* Email cards */}
          {loading && !emails ? (
            <div className="flex items-center justify-center py-8">
              <Spinner size="md" variant="muted" />
            </div>
          ) : (
            <>
              {visibleEmails.map((e) => (
                <EmailCard
                  key={e.email}
                  email={e}
                  sending={sendingCode === e.email}
                  verifying={verifyingEmail === e.email}
                  code={verifyingEmail === e.email ? code : ''}
                  retryAfter={verifyingEmail === e.email ? retryAfter : 0}
                  onCodeChange={setCode}
                  onSendCode={sendCode}
                  onVerifyCode={verifyCode}
                  onRemove={removeEmail}
                  busy={busy}
                />
              ))}
              {hasVerified && visibleEmails.length === 0 && (
                <VerifiedEmailCard />
              )}
              {/* Prompt to send code if there's an unverified email but user
                  hasn't clicked send yet. */}
              {showSendHint && <SendCodeHint />}
            </>
          )}

          {hasVerified && <ContinueButton />}
          {!hasVerified && <AddEmailCard busy={busy} onAddEmail={addEmail} />}
        </div>
      </div>
    </SessionFrame>
  )
}

/** SendCodeHint points the user at the Send code action. */
function SendCodeHint() {
  return (
    <p className="text-foreground-alt text-center text-xs">
      Click <span className="text-brand font-medium">Send code</span> to receive
      a 6-digit verification code by email.
    </p>
  )
}

// VerifiedEmailCard renders the verified gate state when Onboarding Status has
// advanced before the email-list cache has emitted the matching verified row.
function VerifiedEmailCard() {
  return (
    <div className="border-foreground/20 bg-background-get-started flex items-center gap-3 overflow-hidden rounded-lg border p-4 shadow-lg backdrop-blur-sm">
      <div className="bg-brand/20 flex size-8 shrink-0 items-center justify-center rounded-lg">
        <LuCheck className="text-brand size-4" />
      </div>
      <div className="min-w-0 flex-1">
        <h3 className="text-foreground truncate text-sm font-medium">
          Email verified
        </h3>
        <p className="text-brand text-xs">Verified</p>
      </div>
    </div>
  )
}

/** sendCodeLabel is the Send code button content for an email row. */
function sendCodeLabel(sending: boolean, retryAfter: number): ReactNode {
  if (sending) return <Spinner size="sm" />
  if (retryAfter > 0) return retryAfter + 's'
  return 'Send code'
}

/** resendLabel is the text of the resend link under the code entry. */
function resendLabel(sending: boolean, retryAfter: number): string {
  if (sending) return 'Sending…'
  if (retryAfter > 0) return 'Resend in ' + retryAfter + 's'
  return "Didn't get it? Send again"
}

interface EmailCodeEntryProps {
  addr: string
  code: string
  sending: boolean
  busy: boolean
  retryAfter: number
  onCodeChange: (v: string) => void
  onSendCode: (email: string) => Promise<unknown>
  onVerifyCode: () => Promise<unknown>
}

/** EmailCodeEntry takes the 6-digit code once it has been sent. */
function EmailCodeEntry({
  addr,
  code,
  sending,
  busy,
  retryAfter,
  onCodeChange,
  onSendCode,
  onVerifyCode,
}: EmailCodeEntryProps) {
  const handleCodeInputRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  return (
    <div className="border-foreground/10 space-y-3 border-t px-4 pt-3 pb-4">
      <p className="text-foreground-alt text-xs leading-relaxed">
        We sent a 6-digit code to{' '}
        <strong className="text-foreground">{addr}</strong>. Check your inbox
        and enter it below.
      </p>
      <input
        aria-label="Verification code"
        ref={handleCodeInputRef}
        type="text"
        inputMode="numeric"
        maxLength={6}
        placeholder="000000"
        value={code}
        onChange={(e) => onCodeChange(e.target.value.replace(/\D/g, ''))}
        onKeyDown={(e) => {
          if (e.nativeEvent.isComposing) return
          if (e.key === 'Enter') {
            void onVerifyCode()
          }
        }}
        className={cn(
          inputClass,
          'text-center font-mono text-lg tracking-brand-extra-wide',
        )}
      />
      <button
        type="button"
        onClick={() => void onVerifyCode()}
        disabled={busy || code.length !== 6}
        className={cn(
          'group w-full rounded-md border transition-all duration-300',
          'border-brand/30 bg-brand/10 hover:bg-brand/20',
          'disabled:cursor-not-allowed disabled:opacity-50',
          'flex h-10 items-center justify-center gap-2',
        )}
      >
        <span className="text-foreground text-sm">
          {busy ? 'Verifying…' : 'Verify email'}
        </span>
        {!busy && <LuArrowRight className="text-foreground-alt size-4" />}
      </button>
      <button
        type="button"
        onClick={() => void onSendCode(addr)}
        disabled={sending || busy || retryAfter > 0}
        className="text-foreground-alt hover:text-foreground w-full text-center text-xs transition-colors disabled:opacity-50"
      >
        {resendLabel(sending, retryAfter)}
      </button>
    </div>
  )
}

// EmailCard renders a single email address as a collapsible card matching
// the CloudSetupWizard card style.
function EmailCard({
  email,
  sending,
  verifying,
  code,
  retryAfter,
  onCodeChange,
  onSendCode,
  onVerifyCode,
  onRemove,
  busy,
}: {
  email: EmailInfo
  sending: boolean
  verifying: boolean
  code: string
  retryAfter: number
  onCodeChange: (v: string) => void
  onSendCode: (email: string) => Promise<unknown>
  onVerifyCode: () => Promise<unknown>
  onRemove: (email: string) => Promise<unknown>
  busy: boolean
}) {
  const addr = email.email ?? ''
  const verified = email.verified ?? false
  const primary = email.primary ?? false

  return (
    <div className="border-foreground/20 bg-background-get-started overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
      <div className="flex items-center gap-3 p-4">
        <div
          className={cn(
            'flex size-8 shrink-0 items-center justify-center rounded-lg',
            verified ? 'bg-brand/20' : 'bg-brand/10',
          )}
        >
          {verified ? (
            <LuCheck className="text-brand size-4" />
          ) : (
            <LuMail className="text-brand size-4" />
          )}
        </div>
        <div className="min-w-0 flex-1">
          <h3 className="text-foreground truncate text-sm font-medium">
            {addr}
          </h3>
          {verified ? (
            <p className="text-brand text-xs">Verified</p>
          ) : (
            <p className="text-foreground-alt text-xs">Not yet verified</p>
          )}
        </div>

        {/* Actions */}
        <div className="flex items-center gap-1">
          {!verified && (
            <button
              type="button"
              onClick={() => void onSendCode(addr)}
              disabled={sending || busy || retryAfter > 0}
              className={cn(
                'rounded-md border px-3 py-1.5 text-xs font-medium transition-all duration-200',
                'border-brand/30 bg-brand/10 text-foreground hover:bg-brand/20',
                'disabled:cursor-not-allowed disabled:opacity-50',
              )}
            >
              {sendCodeLabel(sending, retryAfter)}
            </button>
          )}
          {!verified && !primary && (
            <button
              type="button"
              onClick={() => void onRemove(addr)}
              disabled={busy}
              className="text-foreground-alt/50 hover:text-destructive rounded p-1.5 transition-colors disabled:opacity-50"
              title="Remove"
            >
              <LuTrash2 className="size-3.5" />
            </button>
          )}
        </div>
      </div>

      {/* Code entry (expanded when code has been sent) */}
      {verifying && !verified && (
        <EmailCodeEntry
          addr={addr}
          code={code}
          sending={sending}
          busy={busy}
          retryAfter={retryAfter}
          onCodeChange={onCodeChange}
          onSendCode={onSendCode}
          onVerifyCode={onVerifyCode}
        />
      )}
    </div>
  )
}
