import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import {
  LuArrowRight,
  LuCheck,
  LuChevronDown,
  LuDownload,
  LuLock,
  LuLockOpen,
  LuShieldCheck,
} from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import {
  CredentialProofInput,
  inputClass,
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

const setupActionButtonClass = cn(
  'group w-full rounded-md border transition-all duration-300',
  'border-brand/30 bg-brand/10 hover:bg-brand/20',
  'disabled:cursor-not-allowed disabled:opacity-50',
  'flex h-10 items-center justify-center gap-2',
)

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
    <div className="bg-background-landing relative flex flex-1 flex-col items-center overflow-y-auto p-6 outline-none md:p-10">
      <div className="relative z-10 my-auto flex w-full max-w-lg flex-col gap-4">
        <div className="flex flex-col items-center gap-2">
          <AnimatedLogo followMouse={false} />
          <h1 className="mt-2 text-xl font-semibold tracking-wide">
            Your data lives on this device
          </h1>
          <p className="text-foreground-alt text-center text-sm">
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
          className={cn(
            'flex w-full cursor-pointer items-center justify-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300 select-none',
            'border-brand bg-brand/10 text-foreground hover:bg-brand/20',
          )}
        >
          Continue to app
          <LuArrowRight className="size-4" />
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
    <div className="border-foreground/20 bg-background-get-started overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
      <button
        type="button"
        onClick={onToggle}
        className="flex w-full items-center gap-3 p-4"
      >
        <div
          className={cn(
            'flex size-8 shrink-0 items-center justify-center rounded-lg',
            done ? 'bg-brand/20' : 'bg-brand/10',
          )}
        >
          {done ? doneIcon : icon}
        </div>
        <div className="flex-1 text-left">
          <h3 className="text-foreground text-sm font-medium">{title}</h3>
          {done ? (
            <p className="text-brand text-xs">{doneSubtitle}</p>
          ) : (
            <p className="text-foreground-alt text-xs">{subtitle}</p>
          )}
        </div>
        <LuChevronDown
          className={cn(
            'text-foreground-alt size-4 shrink-0 transition-transform duration-200',
            expanded && 'rotate-180',
          )}
        />
      </button>
      {expanded && (
        <div className="border-foreground/10 space-y-3 border-t px-4 pt-3 pb-4">
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
      icon={<LuShieldCheck className="text-brand size-4" />}
      doneIcon={<LuCheck className="text-brand size-4" />}
      title="Download a backup key"
      subtitle="Second way to recover your account"
      doneSubtitle="Backup key saved"
    >
      <p className="text-foreground-alt text-xs leading-relaxed">
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
        className={setupActionButtonClass}
      >
        <LuDownload className="text-foreground size-4" />
        <span className="text-foreground text-sm">
          {wiz.downloading ? 'Generating key…' : 'Download backup .pem'}
        </span>
      </button>
      {wiz.error && <p className="text-destructive text-xs">{wiz.error}</p>}
    </CollapsibleSetupCard>
  )
}

// PinFields collects and confirms the PIN, flagging a confirmation mismatch.
function PinFields({ wiz }: { wiz: SetupWizardState }) {
  const pinInputId = useId()
  const confirmPinInputId = useId()

  return (
    <div className="space-y-3">
      <div>
        <label
          htmlFor={pinInputId}
          className="text-foreground-alt mb-1.5 block text-xs select-none"
        >
          PIN
        </label>
        <input
          id={pinInputId}
          type="password"
          value={wiz.pin}
          onChange={(e) => wiz.setPin(e.target.value)}
          placeholder="Enter PIN"
          className={inputClass}
        />
      </div>
      <div>
        <label
          htmlFor={confirmPinInputId}
          className="text-foreground-alt mb-1.5 block text-xs select-none"
        >
          Confirm PIN
        </label>
        <input
          id={confirmPinInputId}
          type="password"
          value={wiz.confirmPin}
          onChange={(e) => wiz.setConfirmPin(e.target.value)}
          placeholder="Confirm PIN"
          className={cn(
            inputClass,
            wiz.confirmPin.length > 0 &&
              wiz.pin !== wiz.confirmPin &&
              'border-destructive/50',
          )}
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
      icon={<LuLock className="text-brand size-4" />}
      doneIcon={<LuCheck className="text-brand size-4" />}
      title="Set a PIN lock"
      subtitle="Require a PIN each time you open the app"
      doneSubtitle="PIN lock enabled"
    >
      <div className="space-y-2">
        <RadioOption
          selected={wiz.lockMode === 'auto'}
          onSelect={() => wiz.setLockMode('auto')}
          icon={<LuLockOpen className="size-4" />}
          label="Auto-unlock"
          description="Key stored on disk. No PIN needed."
        />
        <RadioOption
          selected={wiz.lockMode === 'pin'}
          onSelect={() => wiz.setLockMode('pin')}
          icon={<LuLock className="size-4" />}
          label="PIN lock"
          description="Enter PIN on each app launch."
        />
      </div>
      {wiz.lockMode === 'pin' && <PinFields wiz={wiz} />}
      {wiz.error && <p className="text-destructive text-xs">{wiz.error}</p>}
      <button
        type="button"
        onClick={() => void handleFinishLock()}
        disabled={wiz.saving}
        className={setupActionButtonClass}
      >
        <span className="text-foreground text-sm">
          {wiz.saving ? 'Saving…' : 'Set lock mode'}
        </span>
        {!wiz.saving && <LuArrowRight className="text-foreground-alt size-4" />}
      </button>
    </CollapsibleSetupCard>
  )
}
