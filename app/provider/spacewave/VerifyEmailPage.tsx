import { useCallback, useState, type ReactNode } from 'react'
import {
  LuArrowRight,
  LuCheck,
  LuChevronDown,
  LuMail,
  LuPlus,
  LuSend,
  LuTrash2,
} from '@s4wave/web/images/icons.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import { credentialInputStyles } from '@s4wave/web/ui/credential/CredentialProofInput.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { SessionFrame } from '@s4wave/app/session/SessionFrame.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { useEmailManagement } from '@s4wave/web/hooks/useEmailManagement.js'
import { SpacewaveOnboardingContext } from '@s4wave/web/contexts/SpacewaveOnboardingContext.js'
import type { EmailInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'
import { groupMarker } from '../../../web/style/relations.stylex.js'

import { fonts as themeFonts } from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    width: '100%',
    cursor: 'pointer',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-brand'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule2: {
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    backgroundColor: themeColors['--color-background-get-started'],
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule3: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule5: { flex: 1, textAlign: 'left' },
  rule6: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule7: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule8: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule9: {
    position: 'relative',
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    alignItems: 'center',
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background-landing'],
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@media (min-width: 48rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
  },
  rule10: {
    position: 'relative',
    zIndex: 10,
    marginBlock: 'auto',
    display: 'flex',
    width: '100%',
    maxWidth: 'var(--container-lg)',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule11: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule12: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
  },
  rule13: {
    textAlign: 'center',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule14: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    paddingBlock: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule15: {
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule16: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand'],
  },
  rule17: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    backgroundColor: themeColors['--color-background-get-started'],
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule18: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
  },
  rule19: { minWidth: 0, flex: 1 },
  rule20: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule21: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-brand'],
  },
  rule22: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule23: { color: themeColors['--color-foreground'] },
  rule24: {
    width: '100%',
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule25: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule26: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
  },
  rule27: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
  },
  rule28: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule29: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
  rule30: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule31: {
    borderRadius: '.25rem',
    padding: `calc(${themeLayout['--spacing']} * 1.5)`,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    opacity: { default: null, ':disabled': 0.5 },
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
  },
  rule4: { rotate: '180deg' },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground'],
  },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
  rule7: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

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
      sx={[stylexStyles3.rule1]}
    >
      Continue
      <LuArrowRight sx={stylexStyles.rule1} />
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
    <div sx={stylexStyles3.rule2}>
      <button
        type="button"
        onClick={() => setExpanded(!expanded)}
        sx={stylexStyles3.rule3}
      >
        <div sx={stylexStyles3.rule4}>
          <LuPlus sx={stylexStyles.rule2} />
        </div>
        <div sx={stylexStyles3.rule5}>
          <h3 sx={stylexStyles3.rule6}>Use a different email</h3>
          <p sx={stylexStyles3.rule7}>Add another address to verify instead</p>
        </div>
        <LuChevronDown
          sx={[stylexStyles.rule3, !!expanded && stylexStyles.rule4]}
        />
      </button>
      {expanded && (
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'border-foreground/10 space-y-3 border-t px-4 pt-3 pb-4'
          }
        >
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
            sx={[credentialInputStyles.input]}
          />
          <button
            type="button"
            onClick={() => void handleAdd()}
            disabled={busy || !newEmail}
            sx={[[stylexStyles2.rule1, groupMarker]]}
          >
            <LuSend sx={stylexStyles.rule5} />
            <span sx={stylexStyles3.rule8}>
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
      <div sx={stylexStyles3.rule9}>
        <div sx={stylexStyles3.rule10}>
          {/* Header */}
          <div sx={stylexStyles3.rule11}>
            <AnimatedLogo followMouse={false} />
            <h1 sx={stylexStyles3.rule12}>Verify Your Email</h1>
            <p sx={stylexStyles3.rule13}>
              Confirm your email address to start using Spacewave Cloud.
            </p>
          </div>

          {/* Email cards */}
          {loading && !emails ? (
            <div sx={stylexStyles3.rule14}>
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
    <p sx={stylexStyles3.rule15}>
      Click <span sx={stylexStyles3.rule16}>Send code</span> to receive a
      6-digit verification code by email.
    </p>
  )
}

// VerifiedEmailCard renders the verified gate state when Onboarding Status has
// advanced before the email-list cache has emitted the matching verified row.
function VerifiedEmailCard() {
  return (
    <div sx={stylexStyles3.rule17}>
      <div sx={stylexStyles3.rule18}>
        <LuCheck sx={stylexStyles.rule6} />
      </div>
      <div sx={stylexStyles3.rule19}>
        <h3 sx={stylexStyles3.rule20}>Email verified</h3>
        <p sx={stylexStyles3.rule21}>Verified</p>
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
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'border-foreground/10 space-y-3 border-t px-4 pt-3 pb-4'
      }
    >
      <p sx={stylexStyles3.rule22}>
        We sent a 6-digit code to{' '}
        <strong sx={stylexStyles3.rule23}>{addr}</strong>. Check your inbox and
        enter it below.
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
        sx={[credentialInputStyles.input, credentialInputStyles.code]}
      />
      <button
        type="button"
        onClick={() => void onVerifyCode()}
        disabled={busy || code.length !== 6}
        sx={[[stylexStyles2.rule1, groupMarker]]}
      >
        <span sx={stylexStyles3.rule8}>
          {busy ? 'Verifying…' : 'Verify email'}
        </span>
        {!busy && <LuArrowRight sx={stylexStyles.rule2} />}
      </button>
      <button
        type="button"
        onClick={() => void onSendCode(addr)}
        disabled={sending || busy || retryAfter > 0}
        sx={stylexStyles3.rule24}
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
    <div sx={stylexStyles3.rule2}>
      <div sx={stylexStyles3.rule25}>
        <div
          sx={[
            stylexStyles3.rule26,
            verified ? stylexStyles3.rule27 : stylexStyles3.rule28,
          ]}
        >
          {verified ? (
            <LuCheck sx={stylexStyles.rule6} />
          ) : (
            <LuMail sx={stylexStyles.rule6} />
          )}
        </div>
        <div sx={stylexStyles3.rule19}>
          <h3 sx={stylexStyles3.rule20}>{addr}</h3>
          {verified ? (
            <p sx={stylexStyles3.rule21}>Verified</p>
          ) : (
            <p sx={stylexStyles3.rule7}>Not yet verified</p>
          )}
        </div>

        {/* Actions */}
        <div sx={stylexStyles3.rule29}>
          {!verified && (
            <button
              type="button"
              onClick={() => void onSendCode(addr)}
              disabled={sending || busy || retryAfter > 0}
              sx={[stylexStyles3.rule30]}
            >
              {sendCodeLabel(sending, retryAfter)}
            </button>
          )}
          {!verified && !primary && (
            <button
              type="button"
              onClick={() => void onRemove(addr)}
              disabled={busy}
              sx={stylexStyles3.rule31}
              title="Remove"
            >
              <LuTrash2 sx={stylexStyles.rule7} />
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
