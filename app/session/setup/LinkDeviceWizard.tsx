import type { IconType } from '@s4wave/web/images/icons.js'
import React, { useCallback, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
import {
  LuArrowRight,
  LuCamera,
  LuKeyboard,
  LuMonitor,
  LuSmartphone,
  LuWifi,
} from '@s4wave/web/images/icons.js'

import { DirectAnswerStep } from './DirectAnswerStep.js'
import { DirectOfferStep } from './DirectOfferStep.js'
import { EnterCodeStep } from './EnterCodeStep.js'
import { LinkDeviceDoneStep } from './LinkDeviceDoneStep.js'
import { PairingStep } from './PairingStep.js'
import { PairingVerificationStep } from './PairingVerificationStep.js'
import type { SessionListEntry } from '@s4wave/core/session/session.pb.js'
import { ExternalLink } from '@s4wave/app/landing/ExternalLink.js'
import { SetupPageLayout } from './SetupPageLayout.js'
import {
  useNavigate,
  useParentPaths,
  usePath,
} from '@s4wave/web/router/router.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useOptionalLocalSessionOnboardingContext } from '@s4wave/app/session/setup/LocalSessionOnboardingContext.js'
import { SPACEWAVE_PUBLIC_BASE_URL } from '@s4wave/app/urls.js'
import type { Session } from '@s4wave/sdk/session/session.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'
import { groupMarker } from '../../../web/style/relations.stylex.js'

import { fonts as themeFonts } from '../../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 12)`,
    height: `calc(${themeLayout['--spacing']} * 12)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule3: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule4: {
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule5: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 40%, transparent)`,
      },
    },
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
  },
  rule6: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule7: {
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
  },
  rule8: {
    paddingInline: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
  },
  rule9: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    },
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'left',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      },
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
  rule10: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 9)`,
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule11: { minWidth: 0 },
  rule12: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule13: {
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: themeColors['--color-brand'],
    textTransform: 'uppercase',
  },
  rule14: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule15: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule16: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule17: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule18: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule19: {
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles3 = stylex.create({
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
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    marginLeft: 'auto',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-brand'],
  },
})

type LinkStep =
  | 'choose'
  | 'download'
  | 'pairing'
  | 'enter_code'
  | 'direct_offer'
  | 'direct_answer'
  | 'verify'
  | 'done'

export interface LinkDeviceWizardProps {
  exitPath?: string
}

// describeUnsupportedLink returns why device linking cannot start, or null
// when the session supports pairing.
function describeUnsupportedLink(
  error: Error | null | undefined,
  loading: boolean,
  pairingSupported: boolean,
): string | null {
  if (error) return error.message
  if (loading) return 'Loading session pairing capabilities...'
  if (!pairingSupported) {
    return 'Device linking is not available for this provider.'
  }
  return null
}

// LinkDeviceWizard renders the device linking wizard at /setup/link-device.
// Back navigation is owned per step: sub-steps return to the choose step via
// their in-card back control, and the choose step exits via Skip. The wizard
// deliberately renders no outer back chrome so no step shows two Back controls.
export function LinkDeviceWizard({ exitPath }: LinkDeviceWizardProps) {
  const [step, setStep] = useState<LinkStep>('choose')
  const [remotePeerId, setRemotePeerId] = useState<string | null>(null)

  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const {
    error: sessionInfoError,
    loading: sessionInfoLoading,
    providerId,
  } = useSessionInfo(session)
  const navigate = useNavigate()
  const parentPaths = useParentPaths()
  const path = usePath()
  const fallbackExitPath = parentPaths[parentPaths.length - 1] ?? path
  const resolvedExitPath = exitPath ?? fallbackExitPath
  const onboarding = useOptionalLocalSessionOnboardingContext()

  const handleExit = useCallback(() => {
    navigate({ path: resolvedExitPath })
  }, [navigate, resolvedExitPath])

  const handleRemotePeerResolved = useCallback((peerId: string) => {
    setRemotePeerId(peerId)
    setStep('verify')
  }, [])

  const handleDone = useCallback(
    (entry?: SessionListEntry) => {
      if (entry?.sessionIndex != null) {
        navigate({ path: `/u/${entry.sessionIndex}` })
        return
      }
      onboarding?.markProviderChoiceComplete()
      navigate({ path: resolvedExitPath })
    },
    [navigate, onboarding, resolvedExitPath],
  )

  const handleLinkMore = useCallback(
    (entry?: SessionListEntry) => {
      if (entry?.sessionIndex != null) {
        navigate({ path: `/u/${entry.sessionIndex}/setup/link-device` })
        return
      }
      setRemotePeerId(null)
      setStep('choose')
    },
    [navigate],
  )

  const pairingSupported = providerId === 'local' || providerId === 'spacewave'
  const unsupportedMessage = describeUnsupportedLink(
    sessionInfoError,
    sessionInfoLoading,
    pairingSupported,
  )

  return (
    <SetupPageLayout title="Link My Device" showHeader={step === 'choose'}>
      <div>
        {unsupportedMessage ? (
          <UnsupportedLinkStep
            message={unsupportedMessage}
            buttonLabel="Back"
            onDone={handleExit}
          />
        ) : (
          <LinkStepView
            step={step}
            session={session}
            remotePeerId={remotePeerId}
            onStep={setStep}
            onSkip={handleExit}
            onRemotePeerResolved={handleRemotePeerResolved}
            onDone={handleDone}
            onLinkMore={handleLinkMore}
          />
        )}
      </div>
    </SetupPageLayout>
  )
}

interface LinkStepViewProps {
  step: LinkStep
  session: Session | null | undefined
  remotePeerId: string | null
  onStep: (step: LinkStep) => void
  onSkip: () => void
  onRemotePeerResolved: (peerId: string) => void
  onDone: (entry?: SessionListEntry) => void
  onLinkMore: (entry?: SessionListEntry) => void
}

// LinkStepView renders the wizard card for the current step.
function LinkStepView({
  step,
  session,
  remotePeerId,
  onStep,
  onSkip,
  onRemotePeerResolved,
  onDone,
  onLinkMore,
}: LinkStepViewProps) {
  const backToChoose = () => onStep('choose')

  switch (step) {
    case 'choose':
      return (
        <ChooseStep
          onDownload={() => onStep('download')}
          onGenerate={() => onStep('pairing')}
          onEnterCode={() => onStep('enter_code')}
          onDirectOffer={() => onStep('direct_offer')}
          onDirectAnswer={() => onStep('direct_answer')}
          onSkip={onSkip}
        />
      )
    case 'download':
      return (
        <DownloadStep
          onContinue={() => onStep('pairing')}
          onBack={backToChoose}
        />
      )
    case 'pairing':
      return (
        <PairingStep
          session={session}
          onRemotePeerResolved={onRemotePeerResolved}
          onBack={backToChoose}
        />
      )
    case 'enter_code':
      return (
        <EnterCodeStep
          session={session}
          onRemotePeerResolved={onRemotePeerResolved}
          onBack={backToChoose}
        />
      )
    case 'direct_offer':
      return (
        <DirectOfferStep
          session={session}
          onRemotePeerResolved={onRemotePeerResolved}
          onBack={backToChoose}
        />
      )
    case 'direct_answer':
      return (
        <DirectAnswerStep
          session={session}
          onRemotePeerResolved={onRemotePeerResolved}
          onBack={backToChoose}
        />
      )
    case 'verify':
      return (
        <PairingVerificationStep
          session={session}
          onContinue={() => onStep('done')}
          onAbort={backToChoose}
        />
      )
    case 'done':
      return (
        <LinkDeviceDoneStep
          session={session}
          remotePeerId={remotePeerId}
          onDone={onDone}
          onLinkMore={onLinkMore}
        />
      )
  }
}

interface UnsupportedLinkStepProps {
  message: string
  buttonLabel: string
  onDone: () => void
}

function UnsupportedLinkStep({
  message,
  buttonLabel,
  onDone,
}: UnsupportedLinkStepProps) {
  return (
    <div>
      <div sx={[nativeChildSpacing.s4, stylexStyles4.rule1]}>
        <div sx={stylexStyles4.rule2}>
          <LuMonitor sx={stylexStyles.rule1} />
        </div>
        <h2 sx={stylexStyles4.rule3}>Device linking unavailable</h2>
        <p sx={stylexStyles4.rule4}>{message}</p>
      </div>

      <button
        type="button"
        onClick={onDone}
        sx={[nativeChildSpacing.s4, [stylexStyles4.rule5]]}
      >
        <span sx={stylexStyles4.rule6}>{buttonLabel}</span>
        <LuArrowRight sx={stylexStyles.rule2} />
      </button>
    </div>
  )
}

interface ChooseStepProps {
  onDownload: () => void
  onGenerate: () => void
  onEnterCode: () => void
  onDirectOffer: () => void
  onDirectAnswer: () => void
  onSkip: () => void
}

function ChooseStep({
  onDownload,
  onGenerate,
  onEnterCode,
  onDirectOffer,
  onDirectAnswer,
  onSkip,
}: ChooseStepProps) {
  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-5 */ 'space-y-5'
      }
    >
      <p sx={stylexStyles4.rule4}>
        Connect another device to sync your data peer-to-peer.
      </p>

      <OptionGroup label="On this device">
        <ChooseOption
          icon={LuSmartphone}
          label="Generate code for another device"
          description="Show a pairing code for your other device to enter"
          recommended
          onClick={onGenerate}
        />
        <ChooseOption
          icon={LuWifi}
          label="Show QR code"
          description="Display a QR code for your other device to scan"
          onClick={onDirectOffer}
        />
      </OptionGroup>

      <OptionGroup label="On your other device">
        <ChooseOption
          icon={LuKeyboard}
          label="Enter a code from another device"
          description="Type a code shown on your other device"
          onClick={onEnterCode}
        />
        <ChooseOption
          icon={LuCamera}
          label="Scan QR code"
          description="Scan a QR code shown on your other device"
          onClick={onDirectAnswer}
        />
      </OptionGroup>

      {!isDesktop && (
        <ChooseOption
          icon={LuMonitor}
          label="Download desktop app"
          description="Install Spacewave on your computer"
          onClick={onDownload}
        />
      )}

      <button type="button" onClick={onSkip} sx={stylexStyles4.rule7}>
        Skip for now
      </button>
    </div>
  )
}

interface OptionGroupProps {
  label: string
  children: React.ReactNode
}

// OptionGroup labels a set of linking options by which device shows the pairing
// code, so the two capture directions read as distinct paths instead of one
// flat list of near-identical rows.
function OptionGroup({ label, children }: OptionGroupProps) {
  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
      }
    >
      <h3 sx={stylexStyles4.rule8}>{label}</h3>
      {children}
    </div>
  )
}

interface ChooseOptionProps {
  icon: IconType
  label: string
  description: string
  recommended?: boolean
  onClick: () => void
}

function ChooseOption({
  sx,
  icon: Icon,
  label,
  description,
  recommended,
  onClick,
}: ChooseOptionProps & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <button type="button" onClick={onClick} sx={[[stylexStyles4.rule9], sx]}>
      <div sx={stylexStyles4.rule10}>
        <Icon sx={stylexStyles2.rule1} />
      </div>
      <div sx={stylexStyles4.rule11}>
        <div sx={stylexStyles4.rule12}>
          <span sx={stylexStyles4.rule3}>{label}</span>
          {recommended && <span sx={stylexStyles4.rule13}>Recommended</span>}
        </div>
        <p sx={stylexStyles4.rule14}>{description}</p>
      </div>
      <LuArrowRight sx={stylexStyles.rule3} />
    </button>
  )
}

interface DownloadStepProps {
  onContinue: () => void
  onBack: () => void
}

function DownloadStep({ onContinue, onBack }: DownloadStepProps) {
  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-4 */ 'space-y-4'
      }
    >
      <div sx={stylexStyles4.rule15}>
        <div sx={stylexStyles4.rule16}>
          <LuMonitor sx={stylexStyles.rule4} />
        </div>
        <div>
          <h2 sx={stylexStyles4.rule3}>Download the desktop app</h2>
          <p sx={stylexStyles4.rule17}>
            Choose the installer for your computer, then return here to link
            your account.
          </p>
        </div>
      </div>

      <div
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
        }
      >
        <ExternalLink
          href={`${SPACEWAVE_PUBLIC_BASE_URL}/#/download`}
          {...stylex.props(stylexStyles4.rule18)}
        >
          Choose a desktop download
        </ExternalLink>
        <p sx={stylexStyles4.rule19}>
          macOS, Windows, and Linux. Opens in a new tab.
        </p>
      </div>

      <button
        type="button"
        onClick={onContinue}
        sx={[[stylexStyles3.rule1, groupMarker]]}
      >
        <span sx={stylexStyles4.rule6}>I have the desktop app</span>
        <LuArrowRight sx={stylexStyles.rule2} />
      </button>

      <button type="button" onClick={onBack} sx={stylexStyles4.rule7}>
        Back
      </button>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
