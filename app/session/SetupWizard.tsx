import { useCallback, useEffect, useRef, useState } from 'react'
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
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import { useNavigate, useParams } from '@s4wave/web/router/router.js'
import { useSetupWizard } from '@s4wave/app/session/setup/useSetupWizard.js'
import { CloudSetupWizard } from '@s4wave/app/provider/spacewave/CloudSetupWizard.js'
import { WarningCard } from '@s4wave/app/session/setup/LocalSessionSetup.js'
import { useDownloadDesktopApp } from '@s4wave/app/download/handler.js'
import { useSessionOnboardingState } from '@s4wave/app/session/setup/LocalSessionOnboardingContext.js'
import { completeAndDismissLocalSessionOnboardingProviderChoice } from '@s4wave/app/session/setup/local-session-onboarding-state.js'

import type { SetupWizardState } from '@s4wave/app/session/setup/useSetupWizard.js'

// SetupWizard dispatches to the cloud or local variant based on
// the session's provider ID.
export function SetupWizard() {
  const navigate = useNavigate()
  const params = useParams()
  const isReturning = params['*'] === 'returning'
  const exitPath = isReturning ? '../../' : '../'

  const wiz = useSetupWizard()

  if (wiz.providerId === 'spacewave' && !isReturning) {
    return (
      <CloudSetupWizard wiz={wiz} exitPath={exitPath} navigate={navigate} />
    )
  }
  return <LocalSetupWizard wiz={wiz} exitPath={exitPath} navigate={navigate} />
}

// LocalSetupWizard renders a single page for local session setup.
// Matches the cloud wizard pattern: top card with storage warning,
// collapsible backup key and PIN lock cards, continue button.
function LocalSetupWizard({
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
  const downloadDesktopApp = useDownloadDesktopApp()
  const {
    loading: onboardingLoading,
    providerChoiceComplete,
    setOnboarding,
  } = useSessionOnboardingState()
  const requestedProviderChoiceRef = useRef(false)

  // Mark provider complete on mount (user already chose local to get here).
  useEffect(() => {
    if (onboardingLoading) return
    if (providerChoiceComplete) return
    if (requestedProviderChoiceRef.current) return
    requestedProviderChoiceRef.current = true
    setOnboarding(completeAndDismissLocalSessionOnboardingProviderChoice)
  }, [onboardingLoading, providerChoiceComplete, setOnboarding])

  const backupDone = wiz.backupComplete
  const pinDone = wiz.lockComplete

  const handlePemDownloaded = useCallback(async () => {
    const ok = await wiz.handleDownloadPem()
    if (ok) setExpandedCard(null)
  }, [wiz])

  const handleFinishLock = useCallback(async () => {
    await wiz.handleFinishLock()
    setExpandedCard(null)
  }, [wiz])

  return (
    <div className="bg-background-landing relative flex flex-1 flex-col items-center overflow-y-auto p-6 outline-none md:p-10">
      <ShootingStars className="pointer-events-none fixed inset-0 opacity-60" />
      <div className="relative z-10 my-auto flex w-full max-w-lg flex-col gap-4">
        <div className="flex flex-col items-center gap-2">
          <AnimatedLogo followMouse={false} />
          <h1 className="mt-2 text-xl font-bold tracking-wide">
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

        {/* Backup key card */}
        <div className="border-foreground/20 bg-background-get-started overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
          <button
            onClick={() =>
              setExpandedCard(expandedCard === 'backup' ? null : 'backup')
            }
            className="flex w-full items-center gap-3 p-4"
          >
            <div
              className={cn(
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                backupDone ? 'bg-brand/20' : 'bg-brand/10',
              )}
            >
              {backupDone ?
                <LuCheck className="text-brand h-4 w-4" />
              : <LuShieldCheck className="text-brand h-4 w-4" />}
            </div>
            <div className="flex-1 text-left">
              <h3 className="text-foreground text-sm font-medium">
                Download a backup key
              </h3>
              {!backupDone && (
                <p className="text-foreground-alt text-xs">
                  Second way to recover your account
                </p>
              )}
              {backupDone && (
                <p className="text-brand text-xs">Backup key saved</p>
              )}
            </div>
            <LuChevronDown
              className={cn(
                'text-foreground-alt h-4 w-4 shrink-0 transition-transform duration-200',
                expandedCard === 'backup' && 'rotate-180',
              )}
            />
          </button>
          {expandedCard === 'backup' && (
            <div className="border-foreground/10 space-y-3 border-t px-4 pt-3 pb-4">
              <p className="text-foreground-alt text-xs leading-relaxed">
                Choose a recovery password and download a backup key so you have
                two ways to recover this local account later.
              </p>
              <CredentialProofInput
                password={wiz.password}
                onPasswordChange={wiz.setPassword}
                showPem={false}
                passwordLabel="Recovery password"
                passwordPlaceholder="Choose a password for recovery"
              />
              <button
                onClick={() => void handlePemDownloaded()}
                disabled={wiz.downloading || !wiz.accountReady || !wiz.password}
                className={cn(
                  'group w-full rounded-md border transition-all duration-300',
                  'border-brand/30 bg-brand/10 hover:bg-brand/20',
                  'disabled:cursor-not-allowed disabled:opacity-50',
                  'flex h-10 items-center justify-center gap-2',
                )}
              >
                <LuDownload className="text-foreground h-4 w-4" />
                <span className="text-foreground text-sm">
                  {wiz.downloading ?
                    'Generating key...'
                  : 'Download backup .pem'}
                </span>
              </button>
              {wiz.error && (
                <p className="text-destructive text-xs">{wiz.error}</p>
              )}
            </div>
          )}
        </div>

        {/* PIN lock card */}
        <div className="border-foreground/20 bg-background-get-started overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
          <button
            onClick={() =>
              setExpandedCard(expandedCard === 'pin' ? null : 'pin')
            }
            className="flex w-full items-center gap-3 p-4"
          >
            <div
              className={cn(
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                pinDone ? 'bg-brand/20' : 'bg-brand/10',
              )}
            >
              {pinDone ?
                <LuCheck className="text-brand h-4 w-4" />
              : <LuLock className="text-brand h-4 w-4" />}
            </div>
            <div className="flex-1 text-left">
              <h3 className="text-foreground text-sm font-medium">
                Set a PIN lock
              </h3>
              {!pinDone && (
                <p className="text-foreground-alt text-xs">
                  Require a PIN each time you open the app
                </p>
              )}
              {pinDone && (
                <p className="text-brand text-xs">PIN lock enabled</p>
              )}
            </div>
            <LuChevronDown
              className={cn(
                'text-foreground-alt h-4 w-4 shrink-0 transition-transform duration-200',
                expandedCard === 'pin' && 'rotate-180',
              )}
            />
          </button>
          {expandedCard === 'pin' && (
            <div className="border-foreground/10 space-y-3 border-t px-4 pt-3 pb-4">
              <div className="space-y-2">
                <RadioOption
                  selected={wiz.lockMode === 'auto'}
                  onSelect={() => wiz.setLockMode('auto')}
                  icon={<LuLockOpen className="h-4 w-4" />}
                  label="Auto-unlock"
                  description="Key stored on disk. No PIN needed."
                />
                <RadioOption
                  selected={wiz.lockMode === 'pin'}
                  onSelect={() => wiz.setLockMode('pin')}
                  icon={<LuLock className="h-4 w-4" />}
                  label="PIN lock"
                  description="Enter PIN on each app launch."
                />
              </div>
              {wiz.lockMode === 'pin' && (
                <div className="space-y-3">
                  <div>
                    <label className="text-foreground-alt mb-1.5 block text-xs select-none">
                      PIN
                    </label>
                    <input
                      type="password"
                      value={wiz.pin}
                      onChange={(e) => wiz.setPin(e.target.value)}
                      placeholder="Enter PIN"
                      className={inputClass}
                    />
                  </div>
                  <div>
                    <label className="text-foreground-alt mb-1.5 block text-xs select-none">
                      Confirm PIN
                    </label>
                    <input
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
              )}
              {wiz.error && (
                <p className="text-destructive text-xs">{wiz.error}</p>
              )}
              <button
                onClick={() => void handleFinishLock()}
                disabled={wiz.saving}
                className={cn(
                  'group w-full rounded-md border transition-all duration-300',
                  'border-brand/30 bg-brand/10 hover:bg-brand/20',
                  'disabled:cursor-not-allowed disabled:opacity-50',
                  'flex h-10 items-center justify-center gap-2',
                )}
              >
                <span className="text-foreground text-sm">
                  {wiz.saving ? 'Saving...' : 'Set lock mode'}
                </span>
                {!wiz.saving && (
                  <LuArrowRight className="text-foreground-alt h-4 w-4" />
                )}
              </button>
            </div>
          )}
        </div>

        {/* Continue button */}
        <button
          onClick={() => navigate({ path: exitPath })}
          className={cn(
            'flex w-full cursor-pointer items-center justify-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300 select-none',
            'border-brand bg-brand/10 text-foreground hover:bg-brand/20',
          )}
        >
          Continue to app
          <LuArrowRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  )
}
