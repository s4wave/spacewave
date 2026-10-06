import React, { useCallback, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
import {
  LuArrowRight,
  LuCamera,
  LuKeyboard,
  LuMonitor,
  LuSmartphone,
  LuWifi,
} from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
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
    <div className="space-y-4">
      <div className="flex flex-col items-center gap-3">
        <div className="bg-foreground/5 flex size-12 items-center justify-center rounded-full">
          <LuMonitor className="text-foreground-alt size-6" />
        </div>
        <h2 className="text-foreground text-sm font-medium">
          Device linking unavailable
        </h2>
        <p className="text-foreground-alt text-center text-xs leading-relaxed">
          {message}
        </p>
      </div>

      <button
        type="button"
        onClick={onDone}
        className={cn(
          'w-full rounded-md border transition-all duration-300',
          'border-foreground/20 hover:border-foreground/40',
          'flex h-10 items-center justify-center gap-2',
        )}
      >
        <span className="text-foreground text-sm">{buttonLabel}</span>
        <LuArrowRight className="text-foreground-alt size-4" />
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
    <div className="space-y-5">
      <p className="text-foreground-alt text-center text-xs leading-relaxed">
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

      <button
        type="button"
        onClick={onSkip}
        className="text-foreground-alt hover:text-foreground w-full text-center text-xs transition-colors"
      >
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
    <div className="space-y-2">
      <h3 className="text-foreground-alt px-1 text-xs font-medium tracking-wide uppercase">
        {label}
      </h3>
      {children}
    </div>
  )
}

interface ChooseOptionProps {
  icon: React.ComponentType<{ className?: string }>
  label: string
  description: string
  recommended?: boolean
  onClick: () => void
}

function ChooseOption({
  icon: Icon,
  label,
  description,
  recommended,
  onClick,
}: ChooseOptionProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'w-full rounded-md border transition-all duration-300',
        'border-foreground/10 hover:border-brand/30 hover:bg-brand/5',
        'focus-visible:border-brand/30 focus-visible:bg-brand/5 focus-visible:outline-none',
        'flex items-center gap-3 p-3 text-left',
      )}
    >
      <div className="bg-brand/10 flex size-9 shrink-0 items-center justify-center rounded-lg">
        <Icon className="text-brand size-4" />
      </div>
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span className="text-foreground text-sm font-medium">{label}</span>
          {recommended && (
            <span className="bg-brand/15 text-brand micro-ten rounded-full px-1.5 py-0.5 font-medium tracking-wide uppercase">
              Recommended
            </span>
          )}
        </div>
        <p className="text-foreground-alt text-xs">{description}</p>
      </div>
      <LuArrowRight className="text-foreground-alt ml-auto size-4 shrink-0" />
    </button>
  )
}

interface DownloadStepProps {
  onContinue: () => void
  onBack: () => void
}

function DownloadStep({ onContinue, onBack }: DownloadStepProps) {
  return (
    <div className="space-y-4">
      <div className="flex items-start gap-3">
        <div className="bg-brand/10 flex size-10 shrink-0 items-center justify-center rounded-lg">
          <LuMonitor className="text-brand size-5" />
        </div>
        <div>
          <h2 className="text-foreground text-sm font-medium">
            Download the desktop app
          </h2>
          <p className="text-foreground-alt mt-1 text-xs leading-relaxed">
            Choose the installer for your computer, then return here to link
            your account.
          </p>
        </div>
      </div>

      <div className="space-y-2">
        <ExternalLink
          href={`${SPACEWAVE_PUBLIC_BASE_URL}/#/download`}
          className="border-brand/30 bg-brand/10 text-foreground flex h-10 items-center justify-center rounded-md border text-sm"
        >
          Choose a desktop download
        </ExternalLink>
        <p className="text-foreground-alt text-center text-xs">
          macOS, Windows, and Linux. Opens in a new tab.
        </p>
      </div>

      <button
        type="button"
        onClick={onContinue}
        className={cn(
          'group w-full rounded-md border transition-all duration-300',
          'border-brand/30 bg-brand/10 hover:bg-brand/20',
          'flex h-10 items-center justify-center gap-2',
        )}
      >
        <span className="text-foreground text-sm">I have the desktop app</span>
        <LuArrowRight className="text-foreground-alt size-4" />
      </button>

      <button
        type="button"
        onClick={onBack}
        className="text-foreground-alt hover:text-foreground w-full text-center text-xs transition-colors"
      >
        Back
      </button>
    </div>
  )
}
