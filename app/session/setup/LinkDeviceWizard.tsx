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
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule2: {
    borderRadius: '3.40282e38px',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 12)`,
    width: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule4: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    textAlign: 'center',
  },
  rule5: {
    '--tw-duration': '.3s',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 40%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
  },
  rule6: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule7: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule8: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    paddingInline: themeLayout['--spacing'],
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
  },
  rule9: {
    '--tw-duration': '.3s',
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      },
    },
    display: 'flex',
    outlineStyle: { default: null, ':focus-visible': 'none' },
    textAlign: 'left',
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule10: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    width: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule11: { minWidth: 0 },
  rule12: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule13: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    borderRadius: '3.40282e38px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    color: themeColors['--color-brand'],
    fontSize: '10px',
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
  },
  rule14: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule15: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-start',
    display: 'flex',
  },
  rule16: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule17: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: themeLayout['--spacing'],
  },
  rule18: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule19: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
  },
})

const stylexStyles3 = stylex.create({
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
    display: 'flex',
    justifyContent: 'center',
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule2: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    color: themeColors['--color-foreground-alt'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    marginLeft: 'auto',
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
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
    <div>
      <p sx={[nativeChildSpacingNext.s5, stylexStyles4.rule4]}>
        Connect another device to sync your data peer-to-peer.
      </p>

      <OptionGroup sx={nativeChildSpacingNext.s5} label="On this device">
        {(childSx) => (
          <>
            <ChooseOption
              sx={childSx}
              icon={LuSmartphone}
              label="Generate code for another device"
              description="Show a pairing code for your other device to enter"
              recommended
              onClick={onGenerate}
            />
            <ChooseOption
              sx={childSx}
              icon={LuWifi}
              label="Show QR code"
              description="Display a QR code for your other device to scan"
              onClick={onDirectOffer}
            />
          </>
        )}
      </OptionGroup>

      <OptionGroup sx={nativeChildSpacingNext.s5} label="On your other device">
        {(childSx) => (
          <>
            <ChooseOption
              sx={childSx}
              icon={LuKeyboard}
              label="Enter a code from another device"
              description="Type a code shown on your other device"
              onClick={onEnterCode}
            />
            <ChooseOption
              sx={childSx}
              icon={LuCamera}
              label="Scan QR code"
              description="Scan a QR code shown on your other device"
              onClick={onDirectAnswer}
            />
          </>
        )}
      </OptionGroup>

      {!isDesktop && (
        <ChooseOption
          sx={nativeChildSpacingNext.s5}
          icon={LuMonitor}
          label="Download desktop app"
          description="Install Spacewave on your computer"
          onClick={onDownload}
        />
      )}

      <button
        type="button"
        onClick={onSkip}
        sx={[nativeChildSpacingNext.s5, stylexStyles4.rule7]}
      >
        Skip for now
      </button>
    </div>
  )
}

interface OptionGroupProps {
  label: string
  children: (childSx: stylex.StyleXStyles) => React.ReactNode
}

// OptionGroup labels a set of linking options by which device shows the pairing
// code, so the two capture directions read as distinct paths instead of one
// flat list of near-identical rows.
function OptionGroup({
  sx,
  label,
  children,
}: OptionGroupProps & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={[false, sx]}>
      <h3 sx={[setupChildren.child, stylexStyles4.rule8]}>{label}</h3>
      {children(setupChildren.child)}
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
    <div>
      <div sx={[nativeChildSpacingNextNextNext.s4, stylexStyles4.rule15]}>
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

      <div sx={nativeChildSpacingNextNextNext.s4}>
        <ExternalLink
          href={`${SPACEWAVE_PUBLIC_BASE_URL}/#/download`}
          sx={[nativeChildSpacingNextNext.s2, stylexStyles4.rule18]}
        >
          Choose a desktop download
        </ExternalLink>
        <p sx={[nativeChildSpacingNextNext.s2, stylexStyles4.rule19]}>
          macOS, Windows, and Linux. Opens in a new tab.
        </p>
      </div>

      <button
        type="button"
        onClick={onContinue}
        sx={[
          nativeChildSpacingNextNextNext.s4,
          [[stylexStyles3.rule1, groupMarker]],
        ]}
      >
        <span sx={stylexStyles4.rule6}>I have the desktop app</span>
        <LuArrowRight sx={stylexStyles.rule2} />
      </button>

      <button
        type="button"
        onClick={onBack}
        sx={[nativeChildSpacingNextNextNext.s4, stylexStyles4.rule7]}
      >
        Back
      </button>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 5 * var(--tw-space-y-reverse))`,
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
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNextNextNext = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
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
        'calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))',
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)':
        'calc(var(--spacing) * 2 * var(--tw-space-y-reverse))',
    },
  },
})
