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
} from '@s4wave/web/images/icons.js'

import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'
import { credentialInputStyles } from '@s4wave/web/ui/credential/CredentialProofInput.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/persist.js'
import { useEmailManagement } from '@s4wave/web/hooks/useEmailManagement.js'
import type { EmailInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule2: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule3: {
    padding: themeLayout['--spacing'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule4: {
    '--tw-duration': '.15s',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    display: 'flex',
    textAlign: 'left',
    transitionDuration: '.15s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule5: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule6: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule7: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule8: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule9: {
    '--tw-leading': 'var(--leading-tight)',
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-tight)',
  },
  rule10: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'flex-end',
  },
  rule11: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule12: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule13: {
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    minWidth: 0,
  },
  rule14: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule15: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule16: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule17: { flex: 1, minWidth: 0 },
  rule18: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule19: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule20: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-widest)',
    WebkitUserSelect: 'none',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
    display: 'inline-flex',
    flexShrink: 0,
    fontSize: '.55rem',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
    userSelect: 'none',
  },
  rule21: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule22: { opacity: 0.4 },
  rule23: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    fontFamily: themeFonts['--font-mono'],
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule24: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
  },
  rule25: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
  },
  rule26: { color: themeColors['--color-foreground'] },
  rule27: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule28: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule4: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule5: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule6: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: {
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
})

export interface EmailSectionProps {
  open?: boolean
  onOpenChange?: (open: boolean) => void
}

// EmailSection lets the user manage the account's email list after onboarding.
// Mirrors the add / verify / set-primary / remove surface exposed by the
// onboarding VerifyEmailPage, rendered with the SessionDetails design tokens.
export function EmailSection({
  sx,
  open,
  onOpenChange,
}: EmailSectionProps & { sx?: stylex.StyleXStyles; className?: never }) {
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
      sx={sx}
      title="Email"
      icon={<LuMail sx={stylexStyles.rule1} />}
      open={sectionOpen}
      onOpenChange={handleOpenChange}
      badge={
        count > 0 ? <span sx={stylexStyles2.rule1}>{count}</span> : undefined
      }
    >
      {loading && !emails && <p sx={stylexStyles2.rule2}>Loading emails…</p>}
      {!loading && emails && emails.length === 0 && (
        <div sx={stylexStyles2.rule3}>
          <LuMail sx={stylexStyles.rule2} />
          <span>No email addresses yet</span>
        </div>
      )}
      {emails && emails.length > 0 && (
        <div>
          {emails.map((email) => (
            <EmailRow
              sx={nativeChildSpacingNext.s2}
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
  const handleInputRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => onOpenChange(true)}
        sx={stylexStyles2.rule4}
      >
        <div sx={stylexStyles2.rule5}>
          <LuPlus sx={stylexStyles.rule3} />
        </div>
        <span sx={stylexStyles2.rule6}>Add another email</span>
      </button>
    )
  }
  const trimmed = value.trim()
  const canSubmit = !adding && isValidEmail(trimmed)
  return (
    <div sx={stylexStyles4.rule1}>
      <div sx={[nativeChildSpacingNextNext.s2, stylexStyles2.rule7]}>
        <div sx={stylexStyles2.rule5}>
          <LuPlus sx={stylexStyles.rule3} />
        </div>
        <p sx={stylexStyles2.rule8}>Add another email</p>
      </div>
      <input
        ref={handleInputRef}
        type="email"
        autoComplete="email"
        placeholder="you@example.com"
        aria-label="Email address"
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
        sx={[
          nativeChildSpacingNextNext.s2,
          [
            credentialInputStyles.input,
            !!error && credentialInputStyles.invalid,
          ],
        ]}
        aria-invalid={!!error}
        aria-describedby={error ? 'add-email-error' : undefined}
      />
      {error && (
        <p
          id="add-email-error"
          sx={[nativeChildSpacingNextNext.s2, stylexStyles2.rule9]}
        >
          {error}
        </p>
      )}
      <div sx={[nativeChildSpacingNextNext.s2, stylexStyles2.rule10]}>
        <DashboardButton
          icon={<LuX sx={stylexStyles.rule4} />}
          onClick={() => onOpenChange(false)}
          disabled={adding}
        >
          Cancel
        </DashboardButton>
        <DashboardButton
          icon={
            adding ? (
              <Spinner size="sm" />
            ) : (
              <LuArrowRight sx={stylexStyles.rule4} />
            )
          }
          variant="primary"
          disabled={!canSubmit}
          onClick={() => void onSubmit()}
        >
          {adding ? 'Adding…' : 'Send code'}
        </DashboardButton>
      </div>
    </div>
  )
}

// EmailRow renders one email entry in the SessionDetails card style.
function EmailRow({
  sx,
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
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const addr = email.email ?? ''
  const verified = email.verified ?? false
  const primary = email.primary ?? false
  const canSetPrimary = verified && !primary

  return (
    <div
      ref={(node) => registerRef(addr, node)}
      sx={[stylexStyles2.rule11, sx]}
    >
      <div sx={stylexStyles2.rule12}>
        <EmailIdentity
          addr={addr}
          verified={verified}
          primary={primary}
          source={email.source ?? ''}
        />
        <EmailRowActions
          addr={addr}
          verified={verified}
          primary={primary}
          verifiedCount={verifiedCount}
          removing={removing}
          sendingCode={sendingCode}
          verifyingCode={verifyingCode}
          retryAfter={retryAfter}
          settingPrimary={settingPrimary}
          showSetPrimary={canSetPrimary && !primaryConfirmOpen}
          onSendCode={onSendCode}
          onRemove={onRemove}
          onRequestPrimary={onRequestPrimary}
        />
      </div>
      {canSetPrimary && primaryConfirmOpen && (
        <PrimaryConfirmPanel
          addr={addr}
          settingPrimary={settingPrimary}
          onCancel={onCancelPrimary}
          onSetPrimary={onSetPrimary}
        />
      )}
      {verifying && !verified && (
        <VerifyCodePanel
          addr={addr}
          code={code}
          sendingCode={sendingCode}
          verifyingCode={verifyingCode}
          retryAfter={retryAfter}
          onCodeChange={onCodeChange}
          onSendCode={onSendCode}
          onVerifyCode={onVerifyCode}
        />
      )}
    </div>
  )
}

// EmailIdentity renders an email's address, verification state, primary badge,
// and source.
function EmailIdentity({
  addr,
  verified,
  primary,
  source,
}: {
  addr: string
  verified: boolean
  primary: boolean
  source: string
}) {
  return (
    <div sx={stylexStyles2.rule13}>
      <div
        sx={[
          stylexStyles2.rule14,
          verified ? stylexStyles2.rule15 : stylexStyles2.rule16,
        ]}
      >
        {verified ? (
          <LuCheck sx={stylexStyles.rule5} />
        ) : (
          <LuMail sx={stylexStyles.rule6} />
        )}
      </div>
      <div sx={stylexStyles2.rule17}>
        <div sx={stylexStyles2.rule18}>
          <p sx={stylexStyles2.rule19}>{addr}</p>
          {primary && (
            <span sx={stylexStyles2.rule20}>
              <LuStar sx={stylexStyles.rule7} />
              Primary
            </span>
          )}
        </div>
        <div sx={stylexStyles2.rule21}>
          <span>{verified ? 'Verified' : 'Not yet verified'}</span>
          {source && (
            <>
              <span aria-hidden sx={stylexStyles2.rule22}>
                &middot;
              </span>
              <span sx={stylexStyles2.rule23}>{source}</span>
            </>
          )}
        </div>
      </div>
    </div>
  )
}

// EmailRowActions renders the send-code, set-primary, and remove buttons of an
// email row.
function EmailRowActions({
  addr,
  verified,
  primary,
  verifiedCount,
  removing,
  sendingCode,
  verifyingCode,
  retryAfter,
  settingPrimary,
  showSetPrimary,
  onSendCode,
  onRemove,
  onRequestPrimary,
}: {
  addr: string
  verified: boolean
  primary: boolean
  verifiedCount: number
  removing: boolean
  sendingCode: boolean
  verifyingCode: boolean
  retryAfter: number
  settingPrimary: boolean
  showSetPrimary: boolean
  onSendCode: (email: string) => Promise<unknown>
  onRemove: (email: string) => Promise<unknown>
  onRequestPrimary: () => void
}) {
  const lastVerified = verified && verifiedCount <= 1
  const canRemove = !primary && !lastVerified
  const removeReason = primary
    ? 'Primary email cannot be removed'
    : lastVerified
      ? 'Cannot remove the only verified email'
      : null
  const busy = removing || sendingCode || verifyingCode || settingPrimary

  return (
    <div sx={stylexStyles2.rule24}>
      {!verified && (
        <DashboardButton
          icon={
            sendingCode ? (
              <Spinner size="sm" />
            ) : (
              <LuSend sx={stylexStyles.rule4} />
            )
          }
          disabled={busy || retryAfter > 0}
          onClick={() => void onSendCode(addr)}
        >
          {sendingCode
            ? 'Sending…'
            : retryAfter > 0
              ? retryAfter + 's'
              : 'Send code'}
        </DashboardButton>
      )}
      {showSetPrimary && (
        <DashboardButton
          icon={<LuStar sx={stylexStyles.rule4} />}
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
  )
}

// PrimaryConfirmPanel asks the user to confirm making addr the primary email.
function PrimaryConfirmPanel({
  addr,
  settingPrimary,
  onCancel,
  onSetPrimary,
}: {
  addr: string
  settingPrimary: boolean
  onCancel: () => void
  onSetPrimary: (email: string) => Promise<unknown>
}) {
  return (
    <div sx={stylexStyles3.rule1}>
      <p sx={[nativeChildSpacing.s2, stylexStyles2.rule25]}>
        Make <strong sx={stylexStyles2.rule26}>{addr}</strong> the primary email
        for billing and notifications?
      </p>
      <div sx={[nativeChildSpacing.s2, stylexStyles2.rule10]}>
        <DashboardButton
          icon={<LuX sx={stylexStyles.rule4} />}
          onClick={onCancel}
          disabled={settingPrimary}
        >
          Cancel
        </DashboardButton>
        <DashboardButton
          icon={
            settingPrimary ? (
              <Spinner size="sm" />
            ) : (
              <LuStar sx={stylexStyles.rule4} />
            )
          }
          variant="primary"
          disabled={settingPrimary}
          onClick={() => void onSetPrimary(addr)}
        >
          {settingPrimary ? 'Updating…' : 'Set as primary'}
        </DashboardButton>
      </div>
    </div>
  )
}

// VerifyCodePanel collects the 6-digit verification code sent to addr.
function VerifyCodePanel({
  addr,
  code,
  sendingCode,
  verifyingCode,
  retryAfter,
  onCodeChange,
  onSendCode,
  onVerifyCode,
}: {
  addr: string
  code: string
  sendingCode: boolean
  verifyingCode: boolean
  retryAfter: number
  onCodeChange: (code: string) => void
  onSendCode: (email: string) => Promise<unknown>
  onVerifyCode: () => Promise<unknown>
}) {
  const handleCodeInputRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  return (
    <div sx={stylexStyles4.rule2}>
      <p sx={[nativeChildSpacingNextNext.s3, stylexStyles2.rule25]}>
        We sent a 6-digit code to{' '}
        <strong sx={stylexStyles2.rule26}>{addr}</strong>. Check your inbox and
        enter it below.
      </p>
      <input
        ref={handleCodeInputRef}
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
        sx={[
          nativeChildSpacingNextNext.s3,
          [credentialInputStyles.input, credentialInputStyles.compactCode],
        ]}
        aria-label={'Verification code for ' + addr}
      />
      <div sx={[nativeChildSpacingNextNext.s3, stylexStyles2.rule27]}>
        <button
          type="button"
          onClick={() => void onSendCode(addr)}
          disabled={sendingCode || verifyingCode || retryAfter > 0}
          sx={stylexStyles2.rule28}
        >
          {sendingCode
            ? 'Sending…'
            : retryAfter > 0
              ? 'Resend in ' + retryAfter + 's'
              : "Didn't get it? Send again"}
        </button>
        <DashboardButton
          icon={
            verifyingCode ? (
              <Spinner size="sm" />
            ) : (
              <LuArrowRight sx={stylexStyles.rule4} />
            )
          }
          disabled={verifyingCode || code.length !== 6}
          onClick={() => void onVerifyCode()}
        >
          {verifyingCode ? 'Verifying…' : 'Verify email'}
        </DashboardButton>
      </div>
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
      icon={
        removing ? <Spinner size="sm" /> : <LuTrash2 sx={stylexStyles.rule4} />
      }
      disabled={!canRemove || removing}
      variant={canRemove ? 'destructive' : undefined}
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNextNext = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})
