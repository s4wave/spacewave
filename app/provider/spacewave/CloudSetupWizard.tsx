import { useCallback, useId, useState, type ReactNode } from 'react'
import {
  LuArrowRight,
  LuCheck,
  LuChevronDown,
  LuCloud,
  LuDownload,
  LuGlobe,
  LuLock,
  LuLockOpen,
  LuShield,
  LuShieldCheck,
  LuUsers,
  LuZap,
} from '@s4wave/web/images/icons.js'

import {
  CredentialProofInput,
  credentialInputStyles,
} from '@s4wave/web/ui/credential/CredentialProofInput.js'
import { useCredentialProof } from '@s4wave/web/ui/credential/useCredentialProof.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import type { SetupWizardState } from '@s4wave/app/session/setup/useSetupWizard.js'

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
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule4: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontWeight: 'var(--font-weight-semibold)',
  },
  rule5: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
  },
  rule7: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'flex-start',
    display: 'flex',
  },
  rule8: {
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
  rule9: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    width: '100%',
  },
  rule10: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule11: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
  },
  rule12: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule13: { flex: 1, textAlign: 'left' },
  rule14: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule15: {
    color: themeColors['--color-brand'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule16: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
  },
  rule17: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule18: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule19: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule20: {
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
  rule21: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    marginBlock: 'auto',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-lg)',
    width: '100%',
  },
  rule22: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule23: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule24: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'center',
  },
  rule25: {
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
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    color: themeColors['--color-brand'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: {
    '--tw-duration': '.2s',
    color: themeColors['--color-foreground-alt'],
    flexShrink: 0,
    transitionDuration: '.2s',
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: { rotate: '180deg' },
  rule6: {
    color: themeColors['--color-foreground'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule8: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const CLOUD_PERKS = [
  { icon: LuGlobe, text: 'Cloud sync and backup active' },
  { icon: LuUsers, text: 'Shared Spaces with collaborators' },
  { icon: LuZap, text: 'Always-on sync across all devices' },
  { icon: LuShield, text: 'End-to-end encrypted' },
]

/** CloudPerksCard confirms the active subscription and lists what it enables. */
function CloudPerksCard() {
  return (
    <div sx={stylexStyles3.rule1}>
      <div sx={stylexStyles3.rule2}>
        <div sx={stylexStyles3.rule3}>
          <LuCloud sx={stylexStyles.rule1} />
        </div>
        <div>
          <h2 sx={stylexStyles3.rule4}>You are all set!</h2>
          <p sx={stylexStyles3.rule5}>Your cloud subscription is now active.</p>
        </div>
      </div>
      <div sx={stylexStyles3.rule6}>
        {CLOUD_PERKS.map(({ text }) => (
          <div key={text} sx={stylexStyles3.rule7}>
            <LuCheck sx={stylexStyles.rule2} />
            <span sx={stylexStyles3.rule5}>{text}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

interface SetupCardProps {
  icon: ReactNode
  title: string
  hint: string
  doneHint: string
  done: boolean
  expanded: boolean
  onToggle: () => void
  children: (childSx: stylex.StyleXStyles) => React.ReactNode
}

/** SetupCard is a collapsible card whose header shows its completion state. */
function SetupCard({
  icon,
  title,
  hint,
  doneHint,
  done,
  expanded,
  onToggle,
  children,
}: SetupCardProps) {
  return (
    <div sx={stylexStyles3.rule8}>
      <button type="button" onClick={onToggle} sx={stylexStyles3.rule9}>
        <div
          sx={[
            stylexStyles3.rule10,
            done ? stylexStyles3.rule11 : stylexStyles3.rule12,
          ]}
        >
          {done ? <LuCheck sx={stylexStyles.rule3} /> : icon}
        </div>
        <div sx={stylexStyles3.rule13}>
          <h3 sx={stylexStyles3.rule14}>{title}</h3>
          <p sx={done ? stylexStyles3.rule15 : stylexStyles3.rule5}>
            {done ? doneHint : hint}
          </p>
        </div>
        <LuChevronDown
          sx={[stylexStyles.rule4, !!expanded && stylexStyles.rule5]}
        />
      </button>
      {expanded && (
        <div sx={stylexStyles4.rule1}>{children(setupChildren.child)}</div>
      )}
    </div>
  )
}

interface CardActionProps {
  wiz: SetupWizardState
  expanded: boolean
  onToggle: () => void
  onDone: () => void
}

/** BackupKeyCard verifies the user's identity and downloads a backup key. */
function BackupKeyCard({ wiz, expanded, onToggle, onDone }: CardActionProps) {
  const cred = useCredentialProof()

  const handleDownload = useCallback(async () => {
    const ok = await wiz.handleDownloadPem(cred.pemData ?? undefined)
    if (ok) onDone()
  }, [wiz, cred.pemData, onDone])

  return (
    <SetupCard
      icon={<LuShieldCheck sx={stylexStyles.rule3} />}
      title="Download a backup key"
      hint="Second way to recover your account"
      doneHint="Backup key saved"
      done={wiz.backupComplete}
      expanded={expanded}
      onToggle={onToggle}
    >
      {(childSx) => (
        <>
          <p sx={[childSx, stylexStyles3.rule16]}>
            A backup key gives you a second way to recover your account. Verify
            your identity to generate one.
          </p>
          <CredentialProofInput
            sx={childSx}
            password={wiz.password}
            onPasswordChange={wiz.setPassword}
            pemFileName={cred.pemFileName}
            onFileChange={cred.handleFileChange}
            fileInputRef={cred.fileInputRef}
            pemLabel="Existing backup key"
          />
          <button
            type="button"
            onClick={() => void handleDownload()}
            disabled={
              wiz.downloading ||
              !wiz.accountReady ||
              (!wiz.password && !cred.pemData)
            }
            sx={[childSx, [[stylexStyles2.rule1, groupMarker]]]}
          >
            <LuDownload sx={stylexStyles.rule6} />
            <span sx={stylexStyles3.rule17}>
              {wiz.downloading ? 'Generating key…' : 'Download backup .pem'}
            </span>
          </button>
          {wiz.error && <p sx={[childSx, stylexStyles3.rule18]}>{wiz.error}</p>}
        </>
      )}
    </SetupCard>
  )
}

/** PinFields collects the PIN and its confirmation. */
function PinFields({
  sx,
  wiz,
}: { wiz: SetupWizardState } & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const pinInputId = useId()
  const confirmPinInputId = useId()
  const mismatch = wiz.confirmPin.length > 0 && wiz.pin !== wiz.confirmPin

  return (
    <div sx={sx}>
      <div sx={nativeChildSpacing.s3}>
        <label htmlFor={pinInputId} sx={stylexStyles3.rule19}>
          PIN
        </label>
        <input
          id={pinInputId}
          type="password"
          value={wiz.pin}
          onChange={(e) => wiz.setPin(e.target.value)}
          placeholder="Enter PIN"
          sx={[credentialInputStyles.input]}
        />
      </div>
      <div sx={nativeChildSpacing.s3}>
        <label htmlFor={confirmPinInputId} sx={stylexStyles3.rule19}>
          Confirm PIN
        </label>
        <input
          id={confirmPinInputId}
          type="password"
          value={wiz.confirmPin}
          onChange={(e) => wiz.setConfirmPin(e.target.value)}
          placeholder="Confirm PIN"
          sx={[
            credentialInputStyles.input,
            mismatch && credentialInputStyles.invalid,
          ]}
        />
      </div>
    </div>
  )
}

/** PinLockCard chooses between auto-unlock and a PIN lock. */
function PinLockCard({ wiz, expanded, onToggle, onDone }: CardActionProps) {
  const handleFinishLock = useCallback(async () => {
    await wiz.handleFinishLock()
    onDone()
  }, [wiz, onDone])

  return (
    <SetupCard
      icon={<LuLock sx={stylexStyles.rule3} />}
      title="Set a PIN lock"
      hint="Require a PIN each time you open the app"
      doneHint="PIN lock enabled"
      done={wiz.lockComplete}
      expanded={expanded}
      onToggle={onToggle}
    >
      {(childSx) => (
        <>
          <div sx={childSx}>
            <RadioOption
              sx={nativeChildSpacingNext.s2}
              selected={wiz.lockMode === 'auto'}
              onSelect={() => wiz.setLockMode('auto')}
              icon={<LuLockOpen sx={stylexStyles.rule7} />}
              label="Auto-unlock"
              description="Key stored on disk. No PIN needed."
            />
            <RadioOption
              sx={nativeChildSpacingNext.s2}
              selected={wiz.lockMode === 'pin'}
              onSelect={() => wiz.setLockMode('pin')}
              icon={<LuLock sx={stylexStyles.rule7} />}
              label="PIN lock"
              description="Enter PIN on each app launch."
            />
          </div>
          {wiz.lockMode === 'pin' && <PinFields sx={childSx} wiz={wiz} />}
          {wiz.error && <p sx={[childSx, stylexStyles3.rule18]}>{wiz.error}</p>}
          <button
            type="button"
            onClick={() => void handleFinishLock()}
            disabled={wiz.saving}
            sx={[childSx, [[stylexStyles2.rule1, groupMarker]]]}
          >
            <span sx={stylexStyles3.rule17}>
              {wiz.saving ? 'Saving…' : 'Set lock mode'}
            </span>
            {!wiz.saving && <LuArrowRight sx={stylexStyles.rule8} />}
          </button>
        </>
      )}
    </SetupCard>
  )
}

// CloudSetupWizard renders the post-checkout welcome page for cloud sessions.
// Single page with collapsible cards for backup key and PIN lock.
export function CloudSetupWizard({
  wiz,
  exitPath,
  navigate,
}: {
  wiz: SetupWizardState
  exitPath: string
  navigate: (to: { path: string }) => void
}) {
  const [expandedCard, setExpandedCard] = useState<'backup' | 'pin' | null>(
    null,
  )
  const collapse = useCallback(() => setExpandedCard(null), [])

  return (
    <div sx={stylexStyles3.rule20}>
      <div sx={stylexStyles3.rule21}>
        <div sx={stylexStyles3.rule22}>
          <AnimatedLogo followMouse={false} />
          <h1 sx={stylexStyles3.rule23}>Welcome to Spacewave Cloud!</h1>
          <p sx={stylexStyles3.rule24}>
            Your subscription is active. A few optional steps to secure your
            account.
          </p>
        </div>

        <CloudPerksCard />

        <BackupKeyCard
          wiz={wiz}
          expanded={expandedCard === 'backup'}
          onToggle={() =>
            setExpandedCard(expandedCard === 'backup' ? null : 'backup')
          }
          onDone={collapse}
        />
        <PinLockCard
          wiz={wiz}
          expanded={expandedCard === 'pin'}
          onToggle={() =>
            setExpandedCard(expandedCard === 'pin' ? null : 'pin')
          }
          onDone={collapse}
        />

        {/* Continue button */}
        <button
          type="button"
          onClick={() => navigate({ path: exitPath })}
          sx={[stylexStyles3.rule25]}
        >
          Continue to app
          <LuArrowRight sx={stylexStyles.rule7} />
        </button>
      </div>
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

// setupChildren preserves the original spacing through the content slot.
const setupChildren = stylex.create({
  child: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)':
        'calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))',
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)':
        'calc(var(--spacing) * 3 * var(--tw-space-y-reverse))',
    },
  },
})
