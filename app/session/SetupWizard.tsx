import * as ownerStylex from '@stylexjs/stylex'

import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import {
  LuArrowRight,
  LuCheck,
  LuChevronDown,
  LuDownload,
  LuLock,
  LuLockOpen,
  LuShieldCheck,
} from '@s4wave/web/images/icons.js'

import {
  CredentialProofInput,
  credentialInputStyles,
} from '@s4wave/web/ui/credential/CredentialProofInput.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import { useNavigate, useParams } from '@s4wave/web/router/router.js'
import { useSetupWizard } from '@s4wave/app/session/setup/useSetupWizard.js'
import { CloudSetupWizard } from '@s4wave/app/provider/spacewave/CloudSetupWizard.js'
import { WarningCard } from '@s4wave/app/session/setup/LocalSessionSetup.js'
import { useDownloadDesktopApp } from '@s4wave/app/download/handler.js'
import {
  useOptionalLocalSessionOnboardingContext,
  useSessionOnboardingState,
  type LocalSessionOnboardingContextValue,
} from '@s4wave/app/session/setup/LocalSessionOnboardingContext.js'
import { completeAndDismissLocalSessionOnboardingProviderChoice } from '@s4wave/app/session/setup/local-session-onboarding-state.js'

import type { SetupWizardState } from '@s4wave/app/session/setup/useSetupWizard.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

const stylexStyles3 = ownerStylex.create({
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

const stylexStyles2 = ownerStylex.create({
  rule1: {
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
  rule2: {
    position: 'relative',
    zIndex: 10,
    marginBlock: 'auto',
    display: 'flex',
    width: '100%',
    maxWidth: 'var(--container-lg)',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
  },
  rule5: {
    textAlign: 'center',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule6: {
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
  rule7: {
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
  rule8: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule9: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
  },
  rule10: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
  },
  rule11: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule12: { flex: 1, textAlign: 'left' },
  rule13: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule14: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-brand'],
  },
  rule15: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
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
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
  },
  rule3: { rotate: '180deg' },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground'],
  },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
  },
})

// SetupWizard dispatches to the cloud or local variant based on
// the session's provider ID.
export function SetupWizard() {
  const localOnboarding = useOptionalLocalSessionOnboardingContext()
  if (localOnboarding) {
    return <SetupWizardWithOnboarding onboarding={localOnboarding} />
  }

  return <SetupWizardWithOwnedOnboarding />
}

function SetupWizardWithOwnedOnboarding() {
  const onboarding = useSessionOnboardingState()
  return <SetupWizardWithOnboarding onboarding={onboarding} />
}

function SetupWizardWithOnboarding({
  onboarding,
}: {
  onboarding: LocalSessionOnboardingContextValue
}) {
  const navigate = useNavigate()
  const params = useParams()
  const isReturning = params['*'] === 'returning'
  const exitPath = isReturning ? '../../' : '../'

  const wiz = useSetupWizard(onboarding)

  if (wiz.providerId === 'spacewave' && !isReturning) {
    return (
      <CloudSetupWizard wiz={wiz} exitPath={exitPath} navigate={navigate} />
    )
  }
  return (
    <LocalSetupWizard
      wiz={wiz}
      onboarding={onboarding}
      exitPath={exitPath}
      navigate={navigate}
    />
  )
}

type ExpandedCard = 'backup' | 'pin' | null

// useCompleteProviderChoice marks the provider choice complete once the
// onboarding state loads. The user already chose local to reach the wizard.
function useCompleteProviderChoice(
  onboarding: LocalSessionOnboardingContextValue,
) {
  const requestedProviderChoiceRef = useRef(false)
  const {
    loading: onboardingLoading,
    providerChoiceComplete,
    setOnboarding,
  } = onboarding

  useEffect(() => {
    if (onboardingLoading) return
    if (providerChoiceComplete) return
    if (requestedProviderChoiceRef.current) return
    requestedProviderChoiceRef.current = true
    setOnboarding(completeAndDismissLocalSessionOnboardingProviderChoice)
  }, [onboardingLoading, providerChoiceComplete, setOnboarding])
}

// LocalSetupWizard renders a single page for local session setup.
// Matches the cloud wizard pattern: top card with storage warning,
// collapsible backup key and PIN lock cards, continue button.
function LocalSetupWizard({
  wiz,
  onboarding,
  exitPath,
  navigate,
}: {
  wiz: SetupWizardState
  onboarding: LocalSessionOnboardingContextValue
  exitPath: string
  navigate: (to: { path: string }) => void
}) {
  const [expandedCard, setExpandedCard] = useState<ExpandedCard>(null)
  const downloadDesktopApp = useDownloadDesktopApp()
  useCompleteProviderChoice(onboarding)

  const toggle = (card: 'backup' | 'pin') =>
    setExpandedCard(expandedCard === card ? null : card)

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          <AnimatedLogo followMouse={false} />
          <h1 sx={stylexStyles2.rule4}>Your data lives on this device</h1>
          <p sx={stylexStyles2.rule5}>
            Free local storage is ready to use. A few optional steps to secure
            your account.
          </p>
        </div>

        {/* Storage warning card */}
        <WarningCard
          onDownload={downloadDesktopApp}
          onUpgrade={() => navigate({ path: `${exitPath}plan` })}
        />

        <BackupKeyCard
          wiz={wiz}
          expanded={expandedCard === 'backup'}
          onToggle={() => toggle('backup')}
          onDone={() => setExpandedCard(null)}
        />
        <PinLockCard
          wiz={wiz}
          expanded={expandedCard === 'pin'}
          onToggle={() => toggle('pin')}
          onDone={() => setExpandedCard(null)}
        />

        {/* Continue button */}
        <button
          type="button"
          onClick={() => navigate({ path: exitPath })}
          sx={[stylexStyles2.rule6]}
        >
          Continue to app
          <LuArrowRight sx={stylexStyles.rule1} />
        </button>
      </div>
    </div>
  )
}

// CollapsibleSetupCard renders a setup step card with a toggle header showing
// the step's done state, and the body when expanded.
function CollapsibleSetupCard({
  done,
  expanded,
  onToggle,
  icon,
  doneIcon,
  title,
  subtitle,
  doneSubtitle,
  children,
}: {
  done: boolean
  expanded: boolean
  onToggle: () => void
  icon: ReactNode
  doneIcon: ReactNode
  title: string
  subtitle: string
  doneSubtitle: string
  children: ReactNode
}) {
  return (
    <div sx={stylexStyles2.rule7}>
      <button type="button" onClick={onToggle} sx={stylexStyles2.rule8}>
        <div
          sx={[
            stylexStyles2.rule9,
            done ? stylexStyles2.rule10 : stylexStyles2.rule11,
          ]}
        >
          {done ? doneIcon : icon}
        </div>
        <div sx={stylexStyles2.rule12}>
          <h3 sx={stylexStyles2.rule13}>{title}</h3>
          {done ? (
            <p sx={stylexStyles2.rule14}>{doneSubtitle}</p>
          ) : (
            <p sx={stylexStyles2.rule15}>{subtitle}</p>
          )}
        </div>
        <LuChevronDown
          sx={[stylexStyles.rule2, !!expanded && stylexStyles.rule3]}
        />
      </button>
      {expanded && (
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'border-foreground/10 space-y-3 border-t px-4 pt-3 pb-4'
          }
        >
          {children}
        </div>
      )}
    </div>
  )
}

// BackupKeyCard collects a recovery password and downloads the backup key.
// onDone collapses the card once the key is downloaded.
function BackupKeyCard({
  wiz,
  expanded,
  onToggle,
  onDone,
}: {
  wiz: SetupWizardState
  expanded: boolean
  onToggle: () => void
  onDone: () => void
}) {
  const handlePemDownloaded = async () => {
    const ok = await wiz.handleDownloadPem()
    if (ok) onDone()
  }

  return (
    <CollapsibleSetupCard
      done={wiz.backupComplete}
      expanded={expanded}
      onToggle={onToggle}
      icon={<LuShieldCheck sx={stylexStyles.rule4} />}
      doneIcon={<LuCheck sx={stylexStyles.rule4} />}
      title="Download a backup key"
      subtitle="Second way to recover your account"
      doneSubtitle="Backup key saved"
    >
      <p sx={stylexStyles2.rule16}>
        Choose a recovery password and download a backup key so you have two
        ways to recover this local account later.
      </p>
      <CredentialProofInput
        password={wiz.password}
        onPasswordChange={wiz.setPassword}
        showPem={false}
        passwordLabel="Recovery password"
        passwordPlaceholder="Choose a password for recovery"
      />
      <button
        type="button"
        onClick={() => void handlePemDownloaded()}
        disabled={wiz.downloading || !wiz.accountReady || !wiz.password}
        sx={[[stylexStyles3.rule1, groupMarker2]]}
      >
        <LuDownload sx={stylexStyles.rule5} />
        <span sx={stylexStyles2.rule17}>
          {wiz.downloading ? 'Generating key…' : 'Download backup .pem'}
        </span>
      </button>
      {wiz.error && <p sx={stylexStyles2.rule18}>{wiz.error}</p>}
    </CollapsibleSetupCard>
  )
}

// PinFields collects and confirms the PIN, flagging a confirmation mismatch.
function PinFields({ wiz }: { wiz: SetupWizardState }) {
  const pinInputId = useId()
  const confirmPinInputId = useId()

  return (
    <div>
      <div sx={nativeChildSpacing.s3}>
        <label htmlFor={pinInputId} sx={stylexStyles2.rule19}>
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
        <label htmlFor={confirmPinInputId} sx={stylexStyles2.rule19}>
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
            wiz.confirmPin.length > 0 &&
              wiz.pin !== wiz.confirmPin &&
              credentialInputStyles.invalid,
          ]}
        />
      </div>
    </div>
  )
}

// PinLockCard chooses between auto-unlock and a PIN lock. onDone collapses the
// card once the lock mode is saved.
function PinLockCard({
  wiz,
  expanded,
  onToggle,
  onDone,
}: {
  wiz: SetupWizardState
  expanded: boolean
  onToggle: () => void
  onDone: () => void
}) {
  const handleFinishLock = async () => {
    await wiz.handleFinishLock()
    onDone()
  }

  return (
    <CollapsibleSetupCard
      done={wiz.lockComplete}
      expanded={expanded}
      onToggle={onToggle}
      icon={<LuLock sx={stylexStyles.rule4} />}
      doneIcon={<LuCheck sx={stylexStyles.rule4} />}
      title="Set a PIN lock"
      subtitle="Require a PIN each time you open the app"
      doneSubtitle="PIN lock enabled"
    >
      <div
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
        }
      >
        <RadioOption
          selected={wiz.lockMode === 'auto'}
          onSelect={() => wiz.setLockMode('auto')}
          icon={<LuLockOpen sx={stylexStyles.rule1} />}
          label="Auto-unlock"
          description="Key stored on disk. No PIN needed."
        />
        <RadioOption
          selected={wiz.lockMode === 'pin'}
          onSelect={() => wiz.setLockMode('pin')}
          icon={<LuLock sx={stylexStyles.rule1} />}
          label="PIN lock"
          description="Enter PIN on each app launch."
        />
      </div>
      {wiz.lockMode === 'pin' && <PinFields wiz={wiz} />}
      {wiz.error && <p sx={stylexStyles2.rule18}>{wiz.error}</p>}
      <button
        type="button"
        onClick={() => void handleFinishLock()}
        disabled={wiz.saving}
        sx={[[stylexStyles3.rule1, groupMarker2]]}
      >
        <span sx={stylexStyles2.rule17}>
          {wiz.saving ? 'Saving…' : 'Set lock mode'}
        </span>
        {!wiz.saving && <LuArrowRight sx={stylexStyles.rule6} />}
      </button>
    </CollapsibleSetupCard>
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
