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

const stylexStyles4 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    paddingBottom: `calc(${themeLayout['--spacing']} * 4)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    '--tw-duration': '.3s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    borderColor: themeColors['--color-brand'],
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
    width: '100%',
  },
  rule2: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: themeColors['--color-background-get-started'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule3: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    width: '100%',
  },
  rule4: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule5: { flex: 1, textAlign: 'left' },
  rule6: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule7: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule8: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule9: {
    '--tw-outline-style': 'none',
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@media (min-width: 48rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    flex: 1,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-landing'],
    display: 'flex',
    flexDirection: 'column',
    outlineStyle: 'none',
    position: 'relative',
    overflowY: 'auto',
  },
  rule10: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    marginBlock: 'auto',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-lg)',
    width: '100%',
  },
  rule11: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule12: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule13: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'center',
  },
  rule14: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 8)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
  },
  rule15: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
  },
  rule16: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule17: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    overflow: 'hidden',
    alignItems: 'center',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: themeColors['--color-background-get-started'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    display: 'flex',
  },
  rule18: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule19: { flex: 1, minWidth: 0 },
  rule20: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule21: {
    color: themeColors['--color-brand'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule22: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
  },
  rule23: { color: themeColors['--color-foreground'] },
  rule24: {
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
    textAlign: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule25: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule26: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule27: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
  },
  rule28: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule29: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
  },
  rule30: {
    '--tw-duration': '.2s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    cursor: { default: null, ':disabled': 'not-allowed' },
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration: '.2s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule31: {
    padding: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '.25rem',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-duration': '.3s',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
    display: 'flex',
    justifyContent: 'center',
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    '--tw-duration': '.2s',
    color: themeColors['--color-foreground-alt'],
    flexShrink: 0,
    transitionDuration: '.2s',
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: { rotate: '180deg' },
  rule5: {
    color: themeColors['--color-foreground'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule6: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
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
        <div sx={stylexStyles4.rule1}>
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
            sx={[nativeChildSpacing.s3, [credentialInputStyles.input]]}
          />
          <button
            type="button"
            onClick={() => void handleAdd()}
            disabled={busy || !newEmail}
            sx={[nativeChildSpacing.s3, [[stylexStyles2.rule1, groupMarker]]]}
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
    <div sx={stylexStyles4.rule1}>
      <p sx={[nativeChildSpacing.s3, stylexStyles3.rule22]}>
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
        sx={[
          nativeChildSpacing.s3,
          [credentialInputStyles.input, credentialInputStyles.code],
        ]}
      />
      <button
        type="button"
        onClick={() => void onVerifyCode()}
        disabled={busy || code.length !== 6}
        sx={[nativeChildSpacing.s3, [[stylexStyles2.rule1, groupMarker]]]}
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
        sx={[nativeChildSpacing.s3, stylexStyles3.rule24]}
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
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
