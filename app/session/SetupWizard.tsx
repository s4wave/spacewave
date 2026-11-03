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

const stylexStyles4 = ownerStylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    paddingBottom: `calc(${themeLayout['--spacing']} * 4)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const stylexStyles3 = ownerStylex.create({
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

const stylexStyles2 = ownerStylex.create({
  rule1: {
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
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    marginBlock: 'auto',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-lg)',
    width: '100%',
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule4: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'center',
  },
  rule6: {
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
  rule7: {
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
  rule8: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    width: '100%',
  },
  rule9: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule10: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
  },
  rule11: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule12: { flex: 1, textAlign: 'left' },
  rule13: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule14: {
    color: themeColors['--color-brand'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule15: {
    color: themeColors['--color-foreground-alt'],
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
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    '--tw-duration': '.2s',
    color: themeColors['--color-foreground-alt'],
    flexShrink: 0,
    transitionDuration: '.2s',
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: { rotate: '180deg' },
  rule4: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    color: themeColors['--color-foreground'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule6: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
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
  children: (childSx: stylex.StyleXStyles) => React.ReactNode
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
        <div sx={stylexStyles4.rule1}>{children(setupChildren.child)}</div>
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
      {(childSx) => (
        <>
          <p sx={[childSx, stylexStyles2.rule16]}>
            Choose a recovery password and download a backup key so you have two
            ways to recover this local account later.
          </p>
          <CredentialProofInput
            sx={childSx}
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
            sx={[childSx, [[stylexStyles3.rule1, groupMarker2]]]}
          >
            <LuDownload sx={stylexStyles.rule5} />
            <span sx={stylexStyles2.rule17}>
              {wiz.downloading ? 'Generating key…' : 'Download backup .pem'}
            </span>
          </button>
          {wiz.error && <p sx={[childSx, stylexStyles2.rule18]}>{wiz.error}</p>}
        </>
      )}
    </CollapsibleSetupCard>
  )
}

// PinFields collects and confirms the PIN, flagging a confirmation mismatch.
function PinFields({
  sx,
  wiz,
}: { wiz: SetupWizardState } & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const pinInputId = useId()
  const confirmPinInputId = useId()

  return (
    <div sx={sx}>
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
      {(childSx) => (
        <>
          <div sx={childSx}>
            <RadioOption
              sx={nativeChildSpacingNext.s2}
              selected={wiz.lockMode === 'auto'}
              onSelect={() => wiz.setLockMode('auto')}
              icon={<LuLockOpen sx={stylexStyles.rule1} />}
              label="Auto-unlock"
              description="Key stored on disk. No PIN needed."
            />
            <RadioOption
              sx={nativeChildSpacingNext.s2}
              selected={wiz.lockMode === 'pin'}
              onSelect={() => wiz.setLockMode('pin')}
              icon={<LuLock sx={stylexStyles.rule1} />}
              label="PIN lock"
              description="Enter PIN on each app launch."
            />
          </div>
          {wiz.lockMode === 'pin' && <PinFields sx={childSx} wiz={wiz} />}
          {wiz.error && <p sx={[childSx, stylexStyles2.rule18]}>{wiz.error}</p>}
          <button
            type="button"
            onClick={() => void handleFinishLock()}
            disabled={wiz.saving}
            sx={[childSx, [[stylexStyles3.rule1, groupMarker2]]]}
          >
            <span sx={stylexStyles2.rule17}>
              {wiz.saving ? 'Saving…' : 'Set lock mode'}
            </span>
            {!wiz.saving && <LuArrowRight sx={stylexStyles.rule6} />}
          </button>
        </>
      )}
    </CollapsibleSetupCard>
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
