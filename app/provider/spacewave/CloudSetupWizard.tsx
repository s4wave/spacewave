import { useCallback, useState } from 'react'
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
} from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import {
  CredentialProofInput,
  inputClass,
} from '@s4wave/web/ui/credential/CredentialProofInput.js'
import { useCredentialProof } from '@s4wave/web/ui/credential/useCredentialProof.js'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import type { SetupWizardState } from '@s4wave/app/session/setup/useSetupWizard.js'

const CLOUD_PERKS = [
  { icon: LuGlobe, text: 'Cloud sync and backup active' },
  { icon: LuUsers, text: 'Shared Spaces with collaborators' },
  { icon: LuZap, text: 'Always-on sync across all devices' },
  { icon: LuShield, text: 'End-to-end encrypted' },
]

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
  const cred = useCredentialProof()

  const backupDone = wiz.backupComplete
  const pinDone = wiz.lockComplete

  const handlePemDownloaded = useCallback(async () => {
    const ok = await wiz.handleDownloadPem(cred.pemData ?? undefined)
    if (ok) setExpandedCard(null)
  }, [wiz, cred.pemData])

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
            Welcome to Spacewave Cloud!
          </h1>
          <p className="text-foreground-alt text-center text-sm">
            Your subscription is active. A few optional steps to secure your
            account.
          </p>
        </div>

        {/* Perks card */}
        <div className="border-brand/30 bg-background-card/50 overflow-hidden rounded-lg border p-6 backdrop-blur-sm">
          <div className="mb-4 flex items-center gap-3">
            <div className="bg-brand/10 flex h-10 w-10 items-center justify-center rounded-lg">
              <LuCloud className="text-brand h-5 w-5" />
            </div>
            <div>
              <h2 className="text-foreground font-semibold">
                You are all set!
              </h2>
              <p className="text-foreground-alt text-xs">
                Your cloud subscription is now active.
              </p>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            {CLOUD_PERKS.map(({ text }) => (
              <div key={text} className="flex items-start gap-2">
                <LuCheck className="text-brand mt-0.5 h-3.5 w-3.5 shrink-0" />
                <span className="text-foreground-alt text-xs">{text}</span>
              </div>
            ))}
          </div>
        </div>

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
                A backup key gives you a second way to recover your account.
                Verify your identity to generate one.
              </p>
              <CredentialProofInput
                password={wiz.password}
                onPasswordChange={wiz.setPassword}
                pemFileName={cred.pemFileName}
                onFileChange={cred.handleFileChange}
                fileInputRef={cred.fileInputRef}
                pemLabel="Existing backup key"
              />
              <button
                onClick={() => void handlePemDownloaded()}
                disabled={
                  wiz.downloading ||
                  !wiz.accountReady ||
                  (!wiz.password && !cred.pemData)
                }
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
