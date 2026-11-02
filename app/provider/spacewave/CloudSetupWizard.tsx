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

import { cn } from '@s4wave/web/style/utils.js'
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
    <div className="border-brand/30 bg-background-card/50 overflow-hidden rounded-lg border p-6">
      <div className="mb-4 flex items-center gap-3">
        <div className="bg-brand/10 flex size-10 items-center justify-center rounded-lg">
          <LuCloud sx={stylexStyles.rule1} />
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
            <LuCheck sx={stylexStyles.rule2} />
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
          {done ? <LuCheck sx={stylexStyles.rule3} /> : icon}
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
          sx={[stylexStyles.rule4, !!expanded && stylexStyles.rule5]}
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
      icon={<LuShieldCheck sx={stylexStyles.rule3} />}
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
        sx={[[stylexStyles2.rule1, groupMarker]]}
      >
        <LuDownload sx={stylexStyles.rule6} />
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
          sx={[credentialInputStyles.input]}
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
      <div className="space-y-2">
        <RadioOption
          selected={wiz.lockMode === 'auto'}
          onSelect={() => wiz.setLockMode('auto')}
          icon={<LuLockOpen sx={stylexStyles.rule7} />}
          label="Auto-unlock"
          description="Key stored on disk. No PIN needed."
        />
        <RadioOption
          selected={wiz.lockMode === 'pin'}
          onSelect={() => wiz.setLockMode('pin')}
          icon={<LuLock sx={stylexStyles.rule7} />}
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
        sx={[[stylexStyles2.rule1, groupMarker]]}
      >
        <span className="text-foreground text-sm">
          {wiz.saving ? 'Saving…' : 'Set lock mode'}
        </span>
        {!wiz.saving && <LuArrowRight sx={stylexStyles.rule8} />}
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
          <LuArrowRight sx={stylexStyles.rule7} />
        </button>
      </div>
    </div>
  )
}
