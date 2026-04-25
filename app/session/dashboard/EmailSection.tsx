import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  LuArrowRight,
  LuCheck,
  LuMail,
  LuPlus,
  LuSend,
  LuStar,
  LuTrash2,
  LuX,
} from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'
import { inputClass } from '@s4wave/web/ui/credential/CredentialProofInput.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/persist.js'
import { useEmailManagement } from '@s4wave/web/hooks/useEmailManagement.js'
import type { EmailInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

export interface EmailSectionProps {
  open?: boolean
  onOpenChange?: (open: boolean) => void
}

// EmailSection lets the user manage the account's email list after onboarding.
// Mirrors the add / verify / set-primary / remove surface exposed by the
// onboarding VerifyEmailPage, rendered with the SessionDetails design tokens.
export function EmailSection({ open, onOpenChange }: EmailSectionProps) {
  const ns = useStateNamespace(['session-settings'])
  const [storedOpen, setStoredOpen] = useStateAtom(ns, 'email', false)
  const sectionOpen = open ?? storedOpen
  const handleOpenChange = onOpenChange ?? setStoredOpen

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
    settingPrimary,
    sendCode,
    verifyCode,
    addEmail,
    removeEmail,
    setPrimaryEmail,
  } = useEmailManagement()
  const count = emails?.length ?? 0
  const verifiedCount = useMemo(
    () => emails?.filter((e) => e.verified).length ?? 0,
    [emails],
  )

  const [primaryPromptFor, setPrimaryPromptFor] = useState<string | null>(null)
  const handleSetPrimary = useCallback(
    async (email: string) => {
      const ok = await setPrimaryEmail(email)
      if (ok) setPrimaryPromptFor(null)
    },
    [setPrimaryEmail],
  )

  const [addOpen, setAddOpen] = useState(false)
  const [newEmail, setNewEmail] = useState('')
  const [addError, setAddError] = useState<string | null>(null)
  const focusEmailRef = useRef<string | null>(null)
  const rowRefs = useRef(new Map<string, HTMLDivElement>())

  const registerRowRef = useCallback(
    (addr: string, node: HTMLDivElement | null) => {
      if (node) {
        rowRefs.current.set(addr, node)
      } else {
        rowRefs.current.delete(addr)
      }
    },
    [],
  )

  useEffect(() => {
    const addr = focusEmailRef.current
    if (!addr || !emails) return
    const row = rowRefs.current.get(addr)
    if (!row) return
    focusEmailRef.current = null
    row.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
    const btn = row.querySelector<HTMLButtonElement>('button')
    btn?.focus()
  }, [emails])

  const handleAddEmail = useCallback(async () => {
    const value = newEmail.trim()
    if (!isValidEmail(value)) {
      setAddError('Enter a valid email address')
      return
    }
    setAddError(null)
    const ok = await addEmail(value)
    if (!ok) {
      setAddError('Failed to add email')
      return
    }
    setNewEmail('')
    setAddOpen(false)
    focusEmailRef.current = value
  }, [addEmail, newEmail])

  const handleVerifyCode = useCallback(async () => {
    if (code.length !== 6) return
    await verifyCode()
  }, [verifyCode, code])

  return (
    <CollapsibleSection
      title="Email"
      icon={<LuMail className="h-3.5 w-3.5" />}
      open={sectionOpen}
      onOpenChange={handleOpenChange}
      badge={
        count > 0 ?
          <span className="text-foreground-alt/50 text-[0.55rem]">{count}</span>
        : undefined
      }
    >
      {loading && !emails && (
        <p className="text-foreground-alt/40 text-xs">Loading emails...</p>
      )}
      {!loading && emails && emails.length === 0 && (
        <div className="text-foreground-alt/40 flex items-center gap-2 px-1 py-1 text-xs">
          <LuMail className="h-3.5 w-3.5 shrink-0" />
          <span>No email addresses yet</span>
        </div>
      )}
      {emails && emails.length > 0 && (
        <div className="space-y-2">
          {emails.map((email) => (
            <EmailRow
              key={email.email}
              email={email}
              verifiedCount={verifiedCount}
              removing={removingEmail === email.email}
              sendingCode={sendingCode === email.email}
              verifying={verifyingEmail === email.email}
              verifyingCode={verifyingCode}
              code={verifyingEmail === email.email ? code : ''}
              retryAfter={verifyingEmail === email.email ? retryAfter : 0}
              settingPrimary={settingPrimary === email.email}
              primaryConfirmOpen={primaryPromptFor === email.email}
              onCodeChange={setCode}
              onSendCode={sendCode}
              onVerifyCode={handleVerifyCode}
              onRemove={removeEmail}
              onRequestPrimary={() => setPrimaryPromptFor(email.email ?? null)}
              onCancelPrimary={() => setPrimaryPromptFor(null)}
              onSetPrimary={handleSetPrimary}
              registerRef={registerRowRef}
            />
          ))}
        </div>
      )}
      <AddEmailForm
        open={addOpen}
        value={newEmail}
        error={addError}
        adding={addingEmail}
        onOpenChange={(next) => {
          setAddOpen(next)
          if (!next) {
            setNewEmail('')
            setAddError(null)
          }
        }}
        onChange={(value) => {
          setNewEmail(value)
          if (addError) setAddError(null)
        }}
        onSubmit={handleAddEmail}
      />
    </CollapsibleSection>
  )
}

function isValidEmail(value: string): boolean {
  if (!value) return false
  // Minimal shape check: one @, a dot in the domain, no whitespace.
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)
}

function AddEmailForm({
  open,
  value,
  error,
  adding,
  onOpenChange,
  onChange,
  onSubmit,
}: {
  open: boolean
  value: string
  error: string | null
  adding: boolean
  onOpenChange: (open: boolean) => void
  onChange: (value: string) => void
  onSubmit: () => Promise<unknown>
}) {
  if (!open) {
    return (
      <button
        type="button"
        onClick={() => onOpenChange(true)}
        className="border-foreground/6 hover:border-foreground/12 hover:bg-background-card/50 bg-background-card/30 flex w-full items-center gap-2 rounded-lg border px-3 py-2 text-left transition-all duration-150"
      >
        <div className="bg-foreground/5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md">
          <LuPlus className="text-foreground-alt h-3.5 w-3.5" />
        </div>
        <span className="text-foreground-alt text-xs">Add another email</span>
      </button>
    )
  }
  const trimmed = value.trim()
  const canSubmit = !adding && isValidEmail(trimmed)
  return (
    <div className="border-foreground/6 bg-background-card/30 space-y-2 rounded-lg border px-3 py-2.5">
      <div className="flex items-center gap-2">
        <div className="bg-foreground/5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md">
          <LuPlus className="text-foreground-alt h-3.5 w-3.5" />
        </div>
        <p className="text-foreground text-xs font-medium">Add another email</p>
      </div>
      <input
        type="email"
        autoComplete="email"
        placeholder="you@example.com"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && canSubmit) {
            e.preventDefault()
            void onSubmit()
          }
          if (e.key === 'Escape') {
            e.preventDefault()
            onOpenChange(false)
          }
        }}
        disabled={adding}
        className={cn(inputClass, error && 'border-destructive/50')}
        autoFocus
        aria-invalid={!!error}
        aria-describedby={error ? 'add-email-error' : undefined}
      />
      {error && (
        <p
          id="add-email-error"
          className="text-destructive text-[0.65rem] leading-tight"
        >
          {error}
        </p>
      )}
      <div className="flex items-center justify-end gap-1.5">
        <DashboardButton
          icon={<LuX className="h-3 w-3" />}
          onClick={() => onOpenChange(false)}
          disabled={adding}
        >
          Cancel
        </DashboardButton>
        <DashboardButton
          icon={
            adding ?
              <Spinner size="sm" />
            : <LuArrowRight className="h-3 w-3" />
          }
          className="text-brand hover:bg-brand/10"
          disabled={!canSubmit}
          onClick={() => void onSubmit()}
        >
          {adding ? 'Adding...' : 'Send code'}
        </DashboardButton>
      </div>
    </div>
  )
}

// EmailRow renders one email entry in the SessionDetails card style.
function EmailRow({
  email,
  verifiedCount,
  removing,
  sendingCode,
  verifying,
  verifyingCode,
  code,
  retryAfter,
  settingPrimary,
  primaryConfirmOpen,
  onCodeChange,
  onSendCode,
  onVerifyCode,
  onRemove,
  onRequestPrimary,
  onCancelPrimary,
  onSetPrimary,
  registerRef,
}: {
  email: EmailInfo
  verifiedCount: number
  removing: boolean
  sendingCode: boolean
  verifying: boolean
  verifyingCode: boolean
  code: string
  retryAfter: number
  settingPrimary: boolean
  primaryConfirmOpen: boolean
  onCodeChange: (code: string) => void
  onSendCode: (email: string) => Promise<unknown>
  onVerifyCode: () => Promise<unknown>
  onRemove: (email: string) => Promise<unknown>
  onRequestPrimary: () => void
  onCancelPrimary: () => void
  onSetPrimary: (email: string) => Promise<unknown>
  registerRef: (addr: string, node: HTMLDivElement | null) => void
}) {
  const addr = email.email ?? ''
  const verified = email.verified ?? false
  const primary = email.primary ?? false
  const source = email.source ?? ''
  const lastVerified = verified && verifiedCount <= 1
  const canRemove = !primary && !lastVerified
  const removeReason =
    primary ? 'Primary email cannot be removed'
    : lastVerified ? 'Cannot remove the only verified email'
    : null
  const canSetPrimary = verified && !primary
  const busy = removing || sendingCode || verifyingCode || settingPrimary

  return (
    <div
      ref={(node) => registerRef(addr, node)}
      className="border-foreground/6 bg-background-card/30 overflow-hidden rounded-lg border"
    >
      <div className="flex items-center justify-between gap-3 px-3 py-2.5">
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <div
            className={cn(
              'flex h-8 w-8 shrink-0 items-center justify-center rounded-md',
              verified ? 'bg-brand/10' : 'bg-foreground/5',
            )}
          >
            {verified ?
              <LuCheck className="text-brand h-4 w-4" />
            : <LuMail className="text-foreground-alt h-4 w-4" />}
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-1.5">
              <p className="text-foreground truncate text-sm font-medium">
                {addr}
              </p>
              {primary && (
                <span className="border-brand/30 bg-brand/10 text-brand inline-flex shrink-0 items-center gap-1 rounded-full border px-2 py-0.5 text-[0.55rem] font-semibold tracking-widest uppercase select-none">
                  <LuStar className="h-2.5 w-2.5" />
                  Primary
                </span>
              )}
            </div>
            <div className="text-foreground-alt/50 flex items-center gap-1.5 text-[0.6rem]">
              <span>{verified ? 'Verified' : 'Not yet verified'}</span>
              {source && (
                <>
                  <span aria-hidden className="opacity-40">
                    &middot;
                  </span>
                  <span className="font-mono">{source}</span>
                </>
              )}
            </div>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          {!verified && (
            <DashboardButton
              icon={
                sendingCode ?
                  <Spinner size="sm" />
                : <LuSend className="h-3 w-3" />
              }
              disabled={busy || retryAfter > 0}
              onClick={() => void onSendCode(addr)}
            >
              {sendingCode ?
                'Sending...'
              : retryAfter > 0 ?
                retryAfter + 's'
              : 'Send code'}
            </DashboardButton>
          )}
          {canSetPrimary && !primaryConfirmOpen && (
            <DashboardButton
              icon={<LuStar className="h-3 w-3" />}
              disabled={busy}
              onClick={onRequestPrimary}
            >
              Set primary
            </DashboardButton>
          )}
          <RemoveAction
            canRemove={canRemove}
            removing={removing}
            reason={removeReason}
            onRemove={() => onRemove(addr)}
          />
        </div>
      </div>
      {canSetPrimary && primaryConfirmOpen && (
        <div className="border-brand/20 bg-brand/5 space-y-2 border-t px-3 py-2.5">
          <p className="text-foreground-alt text-xs leading-relaxed">
            Make <strong className="text-foreground">{addr}</strong> the primary
            email for billing and notifications?
          </p>
          <div className="flex items-center justify-end gap-1.5">
            <DashboardButton
              icon={<LuX className="h-3 w-3" />}
              onClick={onCancelPrimary}
              disabled={settingPrimary}
            >
              Cancel
            </DashboardButton>
            <DashboardButton
              icon={
                settingPrimary ?
                  <Spinner size="sm" />
                : <LuStar className="h-3 w-3" />
              }
              className="text-brand hover:bg-brand/10"
              disabled={settingPrimary}
              onClick={() => void onSetPrimary(addr)}
            >
              {settingPrimary ? 'Updating...' : 'Set as primary'}
            </DashboardButton>
          </div>
        </div>
      )}
      {verifying && !verified && (
        <div className="border-foreground/6 space-y-3 border-t px-3 py-3">
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
                e.preventDefault()
                void onVerifyCode()
              }
            }}
            className={cn(
              inputClass,
              'text-center font-mono text-base tracking-[0.3em]',
            )}
            autoFocus
            aria-label={'Verification code for ' + addr}
          />
          <div className="flex items-center justify-between gap-2">
            <button
              type="button"
              onClick={() => void onSendCode(addr)}
              disabled={sendingCode || verifyingCode || retryAfter > 0}
              className="text-foreground-alt hover:text-foreground text-xs transition-colors disabled:opacity-50"
            >
              {sendingCode ?
                'Sending...'
              : retryAfter > 0 ?
                'Resend in ' + retryAfter + 's'
              : "Didn't get it? Send again"}
            </button>
            <DashboardButton
              icon={
                verifyingCode ?
                  <Spinner size="sm" />
                : <LuArrowRight className="h-3 w-3" />
              }
              disabled={verifyingCode || code.length !== 6}
              onClick={() => void onVerifyCode()}
            >
              {verifyingCode ? 'Verifying...' : 'Verify email'}
            </DashboardButton>
          </div>
        </div>
      )}
    </div>
  )
}

function RemoveAction({
  canRemove,
  removing,
  reason,
  onRemove,
}: {
  canRemove: boolean
  removing: boolean
  reason: string | null
  onRemove: () => Promise<unknown>
}) {
  const button = (
    <DashboardButton
      icon={removing ? <Spinner size="sm" /> : <LuTrash2 className="h-3 w-3" />}
      disabled={!canRemove || removing}
      className={cn(canRemove && 'text-destructive hover:bg-destructive/10')}
      onClick={() => void onRemove()}
    >
      Remove
    </DashboardButton>
  )
  if (reason) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span>{button}</span>
        </TooltipTrigger>
        <TooltipContent side="left">{reason}</TooltipContent>
      </Tooltip>
    )
  }
  return button
}
