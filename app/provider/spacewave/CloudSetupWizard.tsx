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
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule2: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule4: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule6: {
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule7: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule8: {
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
  rule9: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule10: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
  },
  rule11: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
  },
  rule12: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule13: { flex: 1, textAlign: 'left' },
  rule14: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule15: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-brand'],
  },
  rule16: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule17: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule18: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule19: {
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule20: {
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
  rule21: {
    position: 'relative',
    zIndex: 10,
    marginBlock: 'auto',
    display: 'flex',
    width: '100%',
    maxWidth: 'var(--container-lg)',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule22: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule23: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
  },
  rule24: {
    textAlign: 'center',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule25: {
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
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-brand'],
  },
  rule2: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-brand'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
  },
  rule5: { rotate: '180deg' },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground'],
  },
  rule7: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule8: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
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
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// setupChildren preserves the original spacing through the content slot.
const setupChildren = stylex.create({
  child: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)':
        'calc(var(--spacing) * 3 * var(--tw-space-y-reverse))',
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)':
        'calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))',
    },
  },
})
