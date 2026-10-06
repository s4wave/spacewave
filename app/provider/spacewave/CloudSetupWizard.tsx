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
} from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import {
  CredentialProofInput,
  inputClass,
} from '@s4wave/web/ui/credential/CredentialProofInput.js'
import { useCredentialProof } from '@s4wave/web/ui/credential/useCredentialProof.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import type { SetupWizardState } from '@s4wave/app/session/setup/useSetupWizard.js'

const CLOUD_PERKS = [
  { icon: LuGlobe, text: 'Cloud sync and backup active' },
  { icon: LuUsers, text: 'Shared Spaces with collaborators' },
  { icon: LuZap, text: 'Always-on sync across all devices' },
  { icon: LuShield, text: 'End-to-end encrypted' },
]

/** CloudPerksCard confirms the active subscription and lists what it enables. */
function CloudPerksCard() {
  return (
    <div className="border-brand/30 bg-background-card/50 overflow-hidden rounded-lg border p-6 backdrop-blur-sm">
      <div className="mb-4 flex items-center gap-3">
        <div className="bg-brand/10 flex size-10 items-center justify-center rounded-lg">
          <LuCloud className="text-brand size-5" />
        </div>
        <div>
          <h2 className="text-foreground font-semibold">You are all set!</h2>
          <p className="text-foreground-alt text-xs">
            Your cloud subscription is now active.
          </p>
        </div>
      </div>
      <div className="grid grid-cols-2 gap-3">
        {CLOUD_PERKS.map(({ text }) => (
          <div key={text} className="flex items-start gap-2">
            <LuCheck className="text-brand mt-0.5 size-3.5 shrink-0" />
            <span className="text-foreground-alt text-xs">{text}</span>
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
  children: ReactNode
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
          {done ? <LuCheck className="text-brand size-4" /> : icon}
        </div>
        <div className="flex-1 text-left">
          <h3 className="text-foreground text-sm font-medium">{title}</h3>
          <p
            className={
              done ? 'text-brand text-xs' : 'text-foreground-alt text-xs'
            }
          >
            {done ? doneHint : hint}
          </p>
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
      icon={<LuShieldCheck className="text-brand size-4" />}
      title="Download a backup key"
      hint="Second way to recover your account"
      doneHint="Backup key saved"
      done={wiz.backupComplete}
      expanded={expanded}
      onToggle={onToggle}
    >
      <p className="text-foreground-alt text-xs leading-relaxed">
        A backup key gives you a second way to recover your account. Verify your
        identity to generate one.
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
        type="button"
        onClick={() => void handleDownload()}
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
        <LuDownload className="text-foreground size-4" />
        <span className="text-foreground text-sm">
          {wiz.downloading ? 'Generating key…' : 'Download backup .pem'}
        </span>
      </button>
      {wiz.error && <p className="text-destructive text-xs">{wiz.error}</p>}
    </SetupCard>
  )
}

/** PinFields collects the PIN and its confirmation. */
function PinFields({ wiz }: { wiz: SetupWizardState }) {
  const pinInputId = useId()
  const confirmPinInputId = useId()
  const mismatch = wiz.confirmPin.length > 0 && wiz.pin !== wiz.confirmPin

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
          className={cn(inputClass, mismatch && 'border-destructive/50')}
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
      icon={<LuLock className="text-brand size-4" />}
      title="Set a PIN lock"
      hint="Require a PIN each time you open the app"
      doneHint="PIN lock enabled"
      done={wiz.lockComplete}
      expanded={expanded}
      onToggle={onToggle}
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
        className={cn(
          'group w-full rounded-md border transition-all duration-300',
          'border-brand/30 bg-brand/10 hover:bg-brand/20',
          'disabled:cursor-not-allowed disabled:opacity-50',
          'flex h-10 items-center justify-center gap-2',
        )}
      >
        <span className="text-foreground text-sm">
          {wiz.saving ? 'Saving…' : 'Set lock mode'}
        </span>
        {!wiz.saving && <LuArrowRight className="text-foreground-alt size-4" />}
      </button>
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
    <div className="bg-background-landing relative flex flex-1 flex-col items-center overflow-y-auto p-6 outline-none md:p-10">
      <div className="relative z-10 my-auto flex w-full max-w-lg flex-col gap-4">
        <div className="flex flex-col items-center gap-2">
          <AnimatedLogo followMouse={false} />
          <h1 className="mt-2 text-xl font-semibold tracking-wide">
            Welcome to Spacewave Cloud!
          </h1>
          <p className="text-foreground-alt text-center text-sm">
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
