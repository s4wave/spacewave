import { useCallback, useState } from 'react'
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
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { SessionFrame } from '@s4wave/app/session/SessionFrame.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { useEmailManagement } from '@s4wave/web/hooks/useEmailManagement.js'
import type { EmailInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

// VerifyEmailPage renders the email verification gate page.
// Shown after checkout when the user has no verified email.
export function VerifyEmailPage() {
  const navigate = useNavigate()

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

  const [addExpanded, setAddExpanded] = useState(false)
  const [newEmail, setNewEmail] = useState('')

  const busy = verifyingCode || addingEmail || removingEmail !== null

  const handleAddEmail = useCallback(async () => {
    if (!newEmail) return
    const ok = await addEmail(newEmail)
    if (!ok) return
    setNewEmail('')
    setAddExpanded(false)
  }, [addEmail, newEmail])

  // Determine flow state from the email list.
  const hasVerified = emails?.some((e) => e.verified) ?? false
  const hasUnverified = emails?.some((e) => !e.verified)

  return (
    <SessionFrame>
      <div className="bg-background-landing relative flex flex-1 flex-col items-center overflow-y-auto p-6 outline-none md:p-10">
        <ShootingStars className="pointer-events-none fixed inset-0 opacity-60" />
        <div className="relative z-10 my-auto flex w-full max-w-lg flex-col gap-4">
          {/* Header */}
          <div className="flex flex-col items-center gap-2">
            <AnimatedLogo followMouse={false} />
            <h1 className="mt-2 text-xl font-bold tracking-wide">
              Verify Your Email
            </h1>
            <p className="text-foreground-alt text-center text-sm">
              Confirm your email address to start using Spacewave Cloud.
            </p>
          </div>

          {/* Email cards */}
          {loading && !emails ?
            <div className="flex items-center justify-center py-8">
              <Spinner size="md" className="text-foreground-alt" />
            </div>
          : <>
              {emails?.map((e) => (
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

              {/* Prompt to send code if there's an unverified email but user
                  hasn't clicked send yet. */}
              {!hasVerified &&
                hasUnverified &&
                !verifyingEmail &&
                !sendingCode && (
                  <p className="text-foreground-alt text-center text-xs">
                    Click{' '}
                    <span className="text-brand font-medium">Send code</span> to
                    receive a 6-digit verification code by email.
                  </p>
                )}
            </>
          }

          {/* Continue button after successful verification */}
          {hasVerified && (
            <button
              onClick={() => navigate({ path: '../' })}
              className={cn(
                'flex w-full cursor-pointer items-center justify-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300 select-none',
                'border-brand bg-brand/10 text-foreground hover:bg-brand/20',
              )}
            >
              Continue
              <LuArrowRight className="h-4 w-4" />
            </button>
          )}

          {/* Add another email (collapsible, hidden after verification) */}
          {!hasVerified && (
            <div className="border-foreground/20 bg-background-get-started overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
              <button
                onClick={() => setAddExpanded(!addExpanded)}
                className="flex w-full items-center gap-3 p-4"
              >
                <div className="bg-foreground/5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg">
                  <LuPlus className="text-foreground-alt h-4 w-4" />
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
                    'text-foreground-alt h-4 w-4 shrink-0 transition-transform duration-200',
                    addExpanded && 'rotate-180',
                  )}
                />
              </button>
              {addExpanded && (
                <div className="border-foreground/10 space-y-3 border-t px-4 pt-3 pb-4">
                  <input
                    type="email"
                    placeholder="you@example.com"
                    value={newEmail}
                    onChange={(e) => setNewEmail(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        void handleAddEmail()
                      }
                    }}
                    className={inputClass}
                    autoFocus
                  />
                  <button
                    onClick={() => void handleAddEmail()}
                    disabled={busy || !newEmail}
                    className={cn(
                      'group w-full rounded-md border transition-all duration-300',
                      'border-brand/30 bg-brand/10 hover:bg-brand/20',
                      'disabled:cursor-not-allowed disabled:opacity-50',
                      'flex h-10 items-center justify-center gap-2',
                    )}
                  >
                    <LuSend className="text-foreground h-4 w-4" />
                    <span className="text-foreground text-sm">
                      {busy ? 'Adding...' : 'Add & send code'}
                    </span>
                  </button>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </SessionFrame>
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
            'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
            verified ? 'bg-brand/20' : 'bg-brand/10',
          )}
        >
          {verified ?
            <LuCheck className="text-brand h-4 w-4" />
          : <LuMail className="text-brand h-4 w-4" />}
        </div>
        <div className="min-w-0 flex-1">
          <h3 className="text-foreground truncate text-sm font-medium">
            {addr}
          </h3>
          {verified ?
            <p className="text-brand text-xs">Verified</p>
          : <p className="text-foreground-alt text-xs">Not yet verified</p>}
        </div>

        {/* Actions */}
        <div className="flex items-center gap-1">
          {!verified && (
            <button
              onClick={() => void onSendCode(addr)}
              disabled={sending || busy || retryAfter > 0}
              className={cn(
                'rounded-md border px-3 py-1.5 text-xs font-medium transition-all duration-200',
                'border-brand/30 bg-brand/10 text-foreground hover:bg-brand/20',
                'disabled:cursor-not-allowed disabled:opacity-50',
              )}
            >
              {sending ?
                <Spinner size="sm" />
              : retryAfter > 0 ?
                retryAfter + 's'
              : 'Send code'}
            </button>
          )}
          {!primary && !verified && (
            <button
              onClick={() => void onRemove(addr)}
              disabled={busy}
              className="text-foreground-alt/50 hover:text-destructive rounded p-1.5 transition-colors disabled:opacity-50"
              title="Remove"
            >
              <LuTrash2 className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
      </div>

      {/* Code entry (expanded when code has been sent) */}
      {verifying && !verified && (
        <div className="border-foreground/10 space-y-3 border-t px-4 pt-3 pb-4">
          <p className="text-foreground-alt text-xs leading-relaxed">
            We sent a 6-digit code to{' '}
            <strong className="text-foreground">{addr}</strong>. Check your
            inbox and enter it below.
          </p>
          <input
            type="text"
            inputMode="numeric"
            maxLength={6}
            placeholder="000000"
            value={code}
            onChange={(e) => onCodeChange(e.target.value.replace(/\D/g, ''))}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                void onVerifyCode()
              }
            }}
            className={cn(
              inputClass,
              'text-center font-mono text-lg tracking-[0.3em]',
            )}
            autoFocus
          />
          <button
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
              {busy ? 'Verifying...' : 'Verify email'}
            </span>
            {!busy && <LuArrowRight className="text-foreground-alt h-4 w-4" />}
          </button>
          <button
            onClick={() => void onSendCode(addr)}
            disabled={sending || busy || retryAfter > 0}
            className="text-foreground-alt hover:text-foreground w-full text-center text-xs transition-colors disabled:opacity-50"
          >
            {sending ?
              'Sending...'
            : retryAfter > 0 ?
              'Resend in ' + retryAfter + 's'
            : "Didn't get it? Send again"}
          </button>
        </div>
      )}
    </div>
  )
}
