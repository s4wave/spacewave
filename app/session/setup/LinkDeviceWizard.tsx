import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
import {
  LuArrowLeft,
  LuArrowRight,
  LuCamera,
  LuCircleCheck,
  LuCopy,
  LuKeyboard,
  LuLink,
  LuMonitor,
  LuRefreshCw,
  LuShieldCheck,
  LuSmartphone,
  LuWifi,
  LuX,
} from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { PhaseChecklist } from './PhaseChecklist.js'
import { LinkDeviceDoneStep } from './LinkDeviceDoneStep.js'
import { SetupPageLayout } from './SetupPageLayout.js'
import {
  useNavigate,
  useParentPaths,
  usePath,
} from '@s4wave/web/router/router.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useLocalSessionOnboardingContext } from '@s4wave/app/session/setup/LocalSessionOnboardingContext.js'
import { PairingStatus } from '@s4wave/sdk/session/session.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import QRCode from 'qrcode'
import { Html5Qrcode } from 'html5-qrcode'

type LinkStep =
  | 'choose'
  | 'download'
  | 'pairing'
  | 'enter_code'
  | 'direct_offer'
  | 'direct_answer'
  | 'verify'
  | 'done'

const CODE_TTL_SECONDS = 600

export interface LinkDeviceWizardProps {
  exitPath?: string
  topLeft?: React.ReactNode
}

// LinkDeviceWizard renders the device linking wizard at /setup/link-device.
export function LinkDeviceWizard({ exitPath, topLeft }: LinkDeviceWizardProps) {
  const [step, setStep] = useState<LinkStep>('choose')
  const [remotePeerId, setRemotePeerId] = useState<string | null>(null)
  const [codeGeneration, setCodeGeneration] = useState(0)

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
  const onboarding = useLocalSessionOnboardingContext()

  const generateCodeCallback = useCallback(
    (signal: AbortSignal) => {
      if (!session || step !== 'pairing') return undefined
      return session.generatePairingCode(signal)
    },
    [session, step, codeGeneration], // eslint-disable-line react-hooks/exhaustive-deps -- codeGeneration triggers re-fetch
  )
  const codeResult = usePromise(generateCodeCallback)

  const handleRegenerateCode = useCallback(() => {
    setCodeGeneration((n) => n + 1)
  }, [])

  const handleBack = useCallback((toStep: LinkStep) => {
    if (toStep === 'pairing') {
      setRemotePeerId(null)
      setCodeGeneration((n) => n + 1)
    }
    setStep(toStep)
  }, [])

  const handleSkip = useCallback(() => {
    navigate({ path: resolvedExitPath })
  }, [navigate, resolvedExitPath])

  const handleRemotePeerResolved = useCallback((peerId: string) => {
    setRemotePeerId(peerId)
    setStep('verify')
  }, [])

  const handleDone = useCallback(() => {
    onboarding.markProviderChoiceComplete()
    navigate({ path: resolvedExitPath })
  }, [navigate, onboarding, resolvedExitPath])
  const handleExit = useCallback(() => {
    navigate({ path: resolvedExitPath })
  }, [navigate, resolvedExitPath])
  const pairingSupported = providerId === 'local'

  return (
    <SetupPageLayout title="Link My Device" topLeft={topLeft}>
      <div className="border-foreground/20 bg-background-get-started relative overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
        <div className="p-6">
          {sessionInfoError && (
            <UnsupportedLinkStep
              message={sessionInfoError.message}
              buttonLabel="Back"
              onDone={handleExit}
            />
          )}

          {!sessionInfoError && sessionInfoLoading && (
            <UnsupportedLinkStep
              message="Loading session pairing capabilities..."
              buttonLabel="Back"
              onDone={handleExit}
            />
          )}

          {!sessionInfoError && !sessionInfoLoading && !pairingSupported && (
            <UnsupportedLinkStep
              message="Device linking is available from local sessions only."
              buttonLabel="Back"
              onDone={handleExit}
            />
          )}

          {!sessionInfoError &&
            !sessionInfoLoading &&
            pairingSupported &&
            step === 'choose' && (
              <ChooseStep
                onDownload={() => setStep('download')}
                onGenerate={() => setStep('pairing')}
                onEnterCode={() => setStep('enter_code')}
                onDirectOffer={() => setStep('direct_offer')}
                onDirectAnswer={() => setStep('direct_answer')}
                onSkip={handleSkip}
              />
            )}

          {!sessionInfoError &&
            !sessionInfoLoading &&
            pairingSupported &&
            step === 'download' && (
              <DownloadStep
                onContinue={() => setStep('pairing')}
                onBack={() => setStep('choose')}
              />
            )}

          {!sessionInfoError &&
            !sessionInfoLoading &&
            pairingSupported &&
            step === 'pairing' && (
              <PairingStep
                session={session}
                code={codeResult.data ?? null}
                loading={codeResult.loading}
                error={codeResult.error ? codeResult.error.message : null}
                generation={codeGeneration}
                onRegenerateCode={handleRegenerateCode}
                onRemotePeerResolved={handleRemotePeerResolved}
                onContinue={() => setStep(remotePeerId ? 'verify' : 'done')}
                onBack={() => handleBack('choose')}
              />
            )}

          {!sessionInfoError &&
            !sessionInfoLoading &&
            pairingSupported &&
            step === 'enter_code' && (
              <EnterCodeStep
                session={session}
                onRemotePeerResolved={handleRemotePeerResolved}
                onBack={() => setStep('choose')}
              />
            )}

          {!sessionInfoError &&
            !sessionInfoLoading &&
            pairingSupported &&
            step === 'direct_offer' && (
              <DirectOfferStep
                session={session}
                onRemotePeerResolved={handleRemotePeerResolved}
                onBack={() => setStep('choose')}
              />
            )}

          {!sessionInfoError &&
            !sessionInfoLoading &&
            pairingSupported &&
            step === 'direct_answer' && (
              <DirectAnswerStep
                session={session}
                onRemotePeerResolved={handleRemotePeerResolved}
                onBack={() => setStep('choose')}
              />
            )}

          {!sessionInfoError &&
            !sessionInfoLoading &&
            pairingSupported &&
            step === 'verify' && (
              <VerifyStep
                session={session}
                remotePeerId={remotePeerId}
                onContinue={() => setStep('done')}
                onAbort={() => handleBack('choose')}
              />
            )}

          {!sessionInfoError &&
            !sessionInfoLoading &&
            pairingSupported &&
            step === 'done' && (
              <LinkDeviceDoneStep
                session={session}
                remotePeerId={remotePeerId}
                onDone={handleDone}
                onLinkMore={() => {
                  setRemotePeerId(null)
                  setStep('choose')
                }}
              />
            )}
        </div>
      </div>
    </SetupPageLayout>
  )
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
        <div className="bg-foreground/5 flex h-12 w-12 items-center justify-center rounded-full">
          <LuMonitor className="text-foreground-alt h-6 w-6" />
        </div>
        <h2 className="text-foreground text-sm font-medium">
          Device linking unavailable
        </h2>
        <p className="text-foreground-alt text-center text-xs leading-relaxed">
          {message}
        </p>
      </div>

      <button
        onClick={onDone}
        className={cn(
          'w-full rounded-md border transition-all duration-300',
          'border-foreground/20 hover:border-foreground/40',
          'flex h-10 items-center justify-center gap-2',
        )}
      >
        <span className="text-foreground text-sm">{buttonLabel}</span>
        <LuArrowRight className="text-foreground-alt h-4 w-4" />
      </button>
    </div>
  )
}

interface ChooseStepProps {
  onDownload?: () => void
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
    <div className="space-y-4">
      <div className="text-center">
        <p className="text-foreground-alt text-xs leading-relaxed">
          Connect another device to sync your data peer-to-peer.
        </p>
      </div>

      <div className="space-y-2">
        <ChooseOption
          icon={LuSmartphone}
          label="Generate code for another device"
          description="Show a pairing code for your other device to enter"
          onClick={onGenerate}
        />
        <ChooseOption
          icon={LuKeyboard}
          label="Enter a code from another device"
          description="Type a code shown on your other device"
          onClick={onEnterCode}
        />
        <ChooseOption
          icon={LuWifi}
          label="Direct connection (show QR)"
          description="Generate a QR code for another device to scan"
          onClick={onDirectOffer}
        />
        <ChooseOption
          icon={LuCamera}
          label="Direct connection (scan QR)"
          description="Scan a QR code from another device"
          onClick={onDirectAnswer}
        />
        {!isDesktop && onDownload && (
          <ChooseOption
            icon={LuMonitor}
            label="Download desktop app"
            description="Install Spacewave on your computer"
            onClick={onDownload}
          />
        )}
      </div>

      <button
        onClick={onSkip}
        className="text-foreground-alt hover:text-foreground w-full text-center text-xs transition-colors"
      >
        Skip for now
      </button>
    </div>
  )
}

interface ChooseOptionProps {
  icon: React.ComponentType<{ className?: string }>
  label: string
  description: string
  onClick: () => void
}

function ChooseOption({
  icon: Icon,
  label,
  description,
  onClick,
}: ChooseOptionProps) {
  return (
    <button
      onClick={onClick}
      className={cn(
        'w-full rounded-md border transition-all duration-300',
        'border-foreground/10 hover:border-brand/30 hover:bg-brand/5',
        'flex items-center gap-3 p-3 text-left',
      )}
    >
      <div className="bg-brand/10 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg">
        <Icon className="text-brand h-4 w-4" />
      </div>
      <div>
        <span className="text-foreground text-sm font-medium">{label}</span>
        <p className="text-foreground-alt text-xs">{description}</p>
      </div>
      <LuArrowRight className="text-foreground-alt ml-auto h-4 w-4 shrink-0" />
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
        <div className="bg-brand/10 flex h-10 w-10 shrink-0 items-center justify-center rounded-lg">
          <LuMonitor className="text-brand h-5 w-5" />
        </div>
        <div>
          <h2 className="text-foreground text-sm font-medium">
            Download the desktop app
          </h2>
          <p className="text-foreground-alt mt-1 text-xs leading-relaxed">
            Download the Spacewave desktop app to keep your data backed up
            locally.
          </p>
        </div>
      </div>

      <div className="space-y-2">
        <PlatformLink label="Download for macOS" />
        <PlatformLink label="Download for Windows" />
        <PlatformLink label="Download for Linux" />
      </div>

      <button
        onClick={onContinue}
        className={cn(
          'group w-full rounded-md border transition-all duration-300',
          'border-brand/30 bg-brand/10 hover:bg-brand/20',
          'flex h-10 items-center justify-center gap-2',
        )}
      >
        <span className="text-foreground text-sm">I have the desktop app</span>
        <LuArrowRight className="text-foreground-alt h-4 w-4" />
      </button>

      <button
        onClick={onBack}
        className="text-foreground-alt hover:text-foreground w-full text-center text-xs transition-colors"
      >
        Back
      </button>
    </div>
  )
}

function PlatformLink({ label }: { label: string }) {
  return (
    <div
      className={cn(
        'border-foreground/10 text-foreground-alt flex h-9 items-center justify-center rounded-md border text-xs',
        'cursor-not-allowed opacity-50',
      )}
    >
      {label} (coming soon)
    </div>
  )
}

// PairingQRCode renders a QR code encoding the deep link URL for the pairing code.
function PairingQRCode({ code }: { code: string }) {
  const qrCallback = useCallback(
    (signal: AbortSignal) => {
      if (!code) return undefined
      const url = `https://spacewave.app/#/pair/${code}`
      return QRCode.toDataURL(url, {
        width: 160,
        margin: 1,
        color: { dark: '#ffffff', light: '#00000000' },
      })
    },
    [code],
  )
  const qrResult = usePromise(qrCallback)

  if (!qrResult.data) return null

  return (
    <div className="flex flex-col items-center gap-1">
      <img
        src={qrResult.data}
        alt="Pairing QR code"
        className="h-40 w-40 rounded"
      />
      <span className="text-foreground-alt text-xs">Or scan this QR code</span>
    </div>
  )
}

interface PairingStepProps {
  session: Session | null | undefined
  code: string | null
  loading: boolean
  error: string | null
  generation: number
  onRegenerateCode: () => void
  onRemotePeerResolved: (peerId: string) => void
  onContinue: () => void
  onBack: () => void
}

function PairingStep({
  session,
  code,
  loading,
  error,
  generation,
  onRegenerateCode,
  onRemotePeerResolved,
  onContinue,
  onBack,
}: PairingStepProps) {
  const formatted = code ? `${code.slice(0, 4)} ${code.slice(4)}` : null

  const [connectionError, setConnectionError] = useState<string | null>(null)

  // Watch pairing status to detect peer connection or transport errors.
  useEffect(() => {
    if (!session || !code) return
    setConnectionError(null)
    const controller = new AbortController()
    ;(async () => {
      for await (const resp of session.watchPairingStatus(controller.signal)) {
        if (controller.signal.aborted) break
        if (
          (resp.status === PairingStatus.PairingStatus_PEER_CONNECTED ||
            resp.status === PairingStatus.PairingStatus_VERIFYING_EMOJI) &&
          resp.remotePeerId
        ) {
          onRemotePeerResolved(resp.remotePeerId)
          break
        }
        if (resp.status === PairingStatus.PairingStatus_SIGNALING_FAILED) {
          setConnectionError(resp.errorMessage || 'Signaling connection failed')
          break
        }
        if (resp.status === PairingStatus.PairingStatus_CONNECTION_TIMEOUT) {
          setConnectionError(resp.errorMessage || 'Connection timed out')
          break
        }
        if (resp.status === PairingStatus.PairingStatus_FAILED) {
          setConnectionError(resp.errorMessage || 'Pairing failed')
          break
        }
      }
    })().catch((err) => {
      if (controller.signal.aborted) return
      setConnectionError(
        err instanceof Error ? err.message : 'Pairing status stream failed',
      )
    })
    return () => controller.abort()
  }, [session, onRemotePeerResolved]) // eslint-disable-line react-hooks/exhaustive-deps -- code not used in effect, avoids spurious stream restarts

  const [elapsed, setElapsed] = useState(0)

  useEffect(() => {
    setElapsed(0)
    if (!code) return
    const interval = setInterval(() => {
      setElapsed((e) => e + 1)
    }, 1000)
    return () => clearInterval(interval)
  }, [generation, code])

  const secondsLeft =
    code ? Math.max(0, CODE_TTL_SECONDS - elapsed) : CODE_TTL_SECONDS

  // Auto-regenerate when the code expires.
  const prevSecondsLeft = useRef(secondsLeft)
  useEffect(() => {
    if (prevSecondsLeft.current > 0 && secondsLeft === 0 && code) {
      onRegenerateCode()
    }
    prevSecondsLeft.current = secondsLeft
  }, [secondsLeft, code, onRegenerateCode])

  const countdown = useMemo(() => {
    const mins = Math.floor(secondsLeft / 60)
    const secs = secondsLeft % 60
    return `${mins}:${secs.toString().padStart(2, '0')}`
  }, [secondsLeft])

  return (
    <div className="space-y-4">
      <div className="text-center">
        <h2 className="text-foreground text-sm font-medium">
          Enter this code in your desktop app
        </h2>
        <p className="text-foreground-alt mt-1 text-xs">
          Open the desktop app and go to Link Session
        </p>
      </div>

      <div className="flex min-h-16 flex-col items-center justify-center gap-3">
        {loading && <Spinner size="lg" className="text-foreground-alt" />}
        {!loading && formatted && (
          <>
            <span className="text-foreground font-mono text-3xl font-bold tracking-[0.3em]">
              {formatted}
            </span>
            <PairingQRCode code={code!} />
          </>
        )}
      </div>

      {!loading && code && !connectionError && (
        <div className="flex flex-col items-center gap-1">
          <div className="flex items-center justify-center gap-2">
            <span className="bg-brand inline-block h-2 w-2 animate-pulse rounded-full" />
            <span className="text-foreground-alt text-xs">
              Waiting for connection...
            </span>
          </div>
          <span
            className={cn(
              'text-xs',
              secondsLeft <= 60 ? 'text-destructive' : 'text-foreground-alt',
            )}
          >
            Code expires in {countdown}
          </span>
        </div>
      )}

      {connectionError && (
        <div className="flex flex-col items-center gap-2">
          <p className="text-destructive text-center text-xs">
            {connectionError}
          </p>
          <button
            onClick={() => {
              setConnectionError(null)
              onRegenerateCode()
            }}
            className={cn(
              'rounded-md border px-3 py-1.5 text-xs transition-all duration-300',
              'border-foreground/20 hover:border-foreground/40',
            )}
          >
            Retry
          </button>
        </div>
      )}

      {error && <p className="text-destructive text-center text-xs">{error}</p>}

      <div className="flex gap-2">
        <button
          onClick={onBack}
          className={cn(
            'rounded-md border transition-all duration-300',
            'border-foreground/20 hover:border-foreground/40',
            'flex h-10 w-10 shrink-0 items-center justify-center',
          )}
        >
          <LuArrowLeft className="text-foreground-alt h-4 w-4" />
        </button>
        <button
          onClick={onRegenerateCode}
          disabled={loading}
          className={cn(
            'flex-1 rounded-md border transition-all duration-300',
            'border-foreground/20 hover:border-foreground/40',
            'disabled:cursor-not-allowed disabled:opacity-50',
            'flex h-10 items-center justify-center gap-2',
          )}
        >
          <LuRefreshCw className="text-foreground-alt h-4 w-4" />
          <span className="text-foreground text-sm">Generate new code</span>
        </button>
      </div>

      {code && (
        <button
          onClick={onContinue}
          className={cn(
            'group w-full rounded-md border transition-all duration-300',
            'border-brand/30 bg-brand/10 hover:bg-brand/20',
            'flex h-10 items-center justify-center gap-2',
          )}
        >
          <span className="text-foreground text-sm">Continue</span>
          <LuArrowRight className="text-foreground-alt h-4 w-4" />
        </button>
      )}
    </div>
  )
}

// extractDirectOfferPayload extracts the raw b58 offer payload from a QR-decoded
// string. The QR may encode a full URL (https://spacewave.app/#/pair/<payload>)
// or just the raw payload. Returns the payload either way.
function extractDirectOfferPayload(decoded: string): string {
  const match = decoded.match(/#\/pair\/(.+)/)
  if (match) return match[1]
  return decoded
}

// extractCodeFromQR extracts a pairing code from a QR-encoded URL or raw code string.
function extractCodeFromQR(decoded: string): string | null {
  const urlMatch = decoded.match(/#\/pair\/([A-Za-z0-9]{8})/)
  if (urlMatch) return urlMatch[1].toUpperCase()
  const cleaned = decoded.replace(/[^A-Za-z0-9]/g, '')
  if (cleaned.length === 8) return cleaned.toUpperCase()
  return null
}

interface QRScannerModalProps {
  onCodeScanned: (code: string) => void
  onClose: () => void
}

// QRScannerModal renders a camera-based QR scanner overlay using html5-qrcode.
function QRScannerModal({ onCodeScanned, onClose }: QRScannerModalProps) {
  const scannerRef = useRef<Html5Qrcode | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!containerRef.current) return

    const scanner = new Html5Qrcode(containerRef.current.id)
    scannerRef.current = scanner

    scanner
      .start(
        { facingMode: 'environment' },
        { fps: 10, qrbox: { width: 200, height: 200 } },
        (decoded) => {
          const code = extractCodeFromQR(decoded)
          if (code) {
            scanner.stop().catch(() => {})
            onCodeScanned(code)
          }
        },
        () => {},
      )
      .catch(() => {})

    return () => {
      scanner.stop().catch(() => {})
    }
  }, [onCodeScanned])

  return (
    <div className="bg-background/80 fixed inset-0 z-50 flex items-center justify-center backdrop-blur-sm">
      <div className="border-foreground/20 bg-background w-full max-w-sm rounded-lg border p-4 shadow-xl">
        <div className="mb-3 flex items-center justify-between">
          <h3 className="text-foreground text-sm font-medium">Scan QR code</h3>
          <button
            onClick={onClose}
            className="text-foreground-alt hover:text-foreground"
          >
            <LuX className="h-4 w-4" />
          </button>
        </div>
        <div
          id="qr-scanner-container"
          ref={containerRef}
          className="overflow-hidden rounded"
        />
        <p className="text-foreground-alt mt-2 text-center text-xs">
          Point your camera at the QR code on your other device.
        </p>
      </div>
    </div>
  )
}

interface CodeInputProps {
  value: string
  onChange: (value: string) => void
  onPaste: (e: React.ClipboardEvent) => void
  onSubmit?: () => void
  disabled?: boolean
}

// CodeInput renders an OTP-style 8-character pairing code input with paste support.
function CodeInput({
  value,
  onChange,
  onPaste,
  onSubmit,
  disabled,
}: CodeInputProps) {
  const formatted =
    value.length > 4 ? `${value.slice(0, 4)} ${value.slice(4)}` : value

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter' && value.length === 8 && !disabled) {
        onSubmit?.()
      }
    },
    [value, disabled, onSubmit],
  )

  return (
    <div className="flex justify-center">
      <input
        type="text"
        value={formatted}
        onChange={(e) => onChange(e.target.value)}
        onPaste={onPaste}
        onKeyDown={handleKeyDown}
        placeholder="XXXX XXXX"
        maxLength={9}
        autoFocus
        disabled={disabled}
        className={cn(
          'border-foreground/20 bg-foreground/5 text-foreground w-48 rounded-md border text-center font-mono text-2xl font-bold tracking-[0.2em]',
          'placeholder:text-foreground/20 focus:border-brand/50 focus:outline-none',
          'disabled:cursor-not-allowed disabled:opacity-50',
          'h-14 px-3',
        )}
      />
    </div>
  )
}

interface EnterCodeStepProps {
  session: Session | null | undefined
  onRemotePeerResolved: (peerId: string) => void
  onBack: () => void
}

function EnterCodeStep({
  session,
  onRemotePeerResolved,
  onBack,
}: EnterCodeStepProps) {
  const [code, setCode] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [scanning, setScanning] = useState(false)

  const handleSubmit = useCallback(async () => {
    if (!session || code.length < 8) return
    setLoading(true)
    setError(null)
    try {
      const remotePeerId = await session.completePairing(
        code.replace(/\s/g, ''),
      )
      if (remotePeerId) {
        onRemotePeerResolved(remotePeerId)
      }
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to complete pairing',
      )
    } finally {
      setLoading(false)
    }
  }, [session, code, onRemotePeerResolved])

  const handleCodeChange = useCallback((value: string) => {
    const cleaned = value
      .replace(/[^A-Za-z0-9]/g, '')
      .toUpperCase()
      .slice(0, 8)
    setCode(cleaned)
  }, [])

  const handlePaste = useCallback((e: React.ClipboardEvent) => {
    e.preventDefault()
    const pasted = e.clipboardData
      .getData('text')
      .replace(/[^A-Za-z0-9]/g, '')
      .toUpperCase()
      .slice(0, 8)
    setCode(pasted)
  }, [])

  const handleCodeScanned = useCallback((scannedCode: string) => {
    setCode(scannedCode)
    setScanning(false)
  }, [])

  return (
    <div className="space-y-4">
      {scanning && (
        <QRScannerModal
          onCodeScanned={handleCodeScanned}
          onClose={() => setScanning(false)}
        />
      )}

      <div className="text-center">
        <div className="mx-auto mb-2 flex h-10 w-10 items-center justify-center">
          <LuKeyboard className="text-brand h-5 w-5" />
        </div>
        <h2 className="text-foreground text-sm font-medium">
          Enter pairing code
        </h2>
        <p className="text-foreground-alt mt-1 text-xs leading-relaxed">
          Enter the 8-character code shown on your other device.
        </p>
      </div>

      <CodeInput
        value={code}
        onChange={handleCodeChange}
        onPaste={handlePaste}
        onSubmit={handleSubmit}
        disabled={loading}
      />

      <button
        onClick={() => setScanning(true)}
        className={cn(
          'w-full rounded-md border transition-all duration-300',
          'border-foreground/10 hover:border-brand/30 hover:bg-brand/5',
          'flex h-9 items-center justify-center gap-2',
        )}
      >
        <LuCamera className="text-foreground-alt h-4 w-4" />
        <span className="text-foreground-alt text-xs">Scan QR code</span>
      </button>

      {error && <p className="text-destructive text-center text-xs">{error}</p>}

      <div className="flex gap-2">
        <button
          onClick={onBack}
          className={cn(
            'rounded-md border transition-all duration-300',
            'border-foreground/20 hover:border-foreground/40',
            'flex h-10 w-10 shrink-0 items-center justify-center',
          )}
        >
          <LuArrowLeft className="text-foreground-alt h-4 w-4" />
        </button>
        <button
          onClick={handleSubmit}
          disabled={loading || code.length < 8 || !session}
          className={cn(
            'flex-1 rounded-md border transition-all duration-300',
            'border-brand/30 bg-brand/10 hover:bg-brand/20',
            'disabled:cursor-not-allowed disabled:opacity-50',
            'flex h-10 items-center justify-center gap-2',
          )}
        >
          {loading ?
            <Spinner />
          : <>
              <LuLink className="text-brand h-4 w-4" />
              <span className="text-foreground text-sm">Connect</span>
            </>
          }
        </button>
      </div>
    </div>
  )
}

interface VerifyStepProps {
  session: Session | null | undefined
  remotePeerId: string | null
  onContinue: () => void
  onAbort: () => void
}

function VerifyStep({
  session,
  remotePeerId,
  onContinue,
  onAbort,
}: VerifyStepProps) {
  const [emoji, setEmoji] = useState<string[] | null>(null)
  const [waitingForRemote, setWaitingForRemote] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Watch pairing status for emoji data and confirmation states.
  useEffect(() => {
    if (!session) return
    const controller = new AbortController()
    ;(async () => {
      for await (const resp of session.watchPairingStatus(controller.signal)) {
        if (controller.signal.aborted) break
        if (
          resp.status === PairingStatus.PairingStatus_VERIFYING_EMOJI &&
          resp.emoji &&
          resp.emoji.length > 0
        ) {
          setEmoji(resp.emoji)
          setWaitingForRemote(false)
          setError(null)
        }
        if (
          resp.status === PairingStatus.PairingStatus_WAITING_FOR_REMOTE_CONFIRM
        ) {
          setWaitingForRemote(true)
        }
        if (resp.status === PairingStatus.PairingStatus_BOTH_CONFIRMED) {
          onContinue()
          break
        }
        if (resp.status === PairingStatus.PairingStatus_PAIRING_REJECTED) {
          setError(resp.errorMessage || 'Pairing was rejected')
          setWaitingForRemote(false)
          break
        }
        if (resp.status === PairingStatus.PairingStatus_CONFIRMATION_TIMEOUT) {
          setError(resp.errorMessage || 'Confirmation timed out')
          setWaitingForRemote(false)
          break
        }
      }
    })().catch((err) => {
      if (controller.signal.aborted) return
      setError(
        err instanceof Error ? err.message : 'Pairing status stream failed',
      )
      setWaitingForRemote(false)
    })
    return () => controller.abort()
  }, [session, onContinue])

  const handleConfirm = useCallback(() => {
    session?.confirmSASMatch(true)
  }, [session])

  const handleReject = useCallback(() => {
    session?.confirmSASMatch(false)
    onAbort()
  }, [session, onAbort])

  if (!remotePeerId && !emoji) {
    return <WaitingForConnection onSkip={onContinue} onAbort={onAbort} />
  }

  if (error) {
    return (
      <div className="space-y-4">
        <div className="flex flex-col items-center gap-2">
          <div className="bg-destructive/10 flex h-10 w-10 items-center justify-center rounded-full">
            <LuX className="text-destructive h-5 w-5" />
          </div>
          <h2 className="text-foreground text-sm font-medium">
            Verification failed
          </h2>
          <p className="text-destructive text-xs">{error}</p>
        </div>
        <button
          onClick={onAbort}
          className={cn(
            'w-full rounded-md border transition-all duration-300',
            'border-foreground/20 hover:border-foreground/40',
            'flex h-10 items-center justify-center gap-2',
          )}
        >
          <LuArrowLeft className="text-foreground-alt h-4 w-4" />
          <span className="text-foreground text-sm">Back to pairing</span>
        </button>
      </div>
    )
  }

  if (waitingForRemote) {
    return (
      <div className="space-y-4">
        <div className="flex flex-col items-center gap-2">
          <div className="bg-brand/10 flex h-10 w-10 items-center justify-center rounded-full">
            <LuShieldCheck className="text-brand h-5 w-5" />
          </div>
          <h2 className="text-foreground text-sm font-medium">
            Waiting for other device
          </h2>
          <p className="text-foreground-alt text-xs leading-relaxed">
            Confirm the emoji match on your other device to continue.
          </p>
        </div>
        {emoji && <EmojiGrid emoji={emoji} />}
        <div className="flex min-h-10 items-center justify-center">
          <Spinner size="lg" className="text-foreground-alt" />
        </div>
      </div>
    )
  }

  if (!emoji) {
    return (
      <div className="space-y-4">
        <div className="flex flex-col items-center gap-2">
          <div className="bg-brand/10 flex h-10 w-10 items-center justify-center rounded-full">
            <LuShieldCheck className="text-brand h-5 w-5" />
          </div>
          <h2 className="text-foreground text-sm font-medium">
            Establishing secure channel
          </h2>
          <p className="text-foreground-alt text-xs leading-relaxed">
            Setting up encrypted connection with your other device...
          </p>
        </div>
        <div className="flex min-h-24 items-center justify-center">
          <Spinner size="lg" className="text-foreground-alt" />
        </div>
        <button
          onClick={onAbort}
          className={cn(
            'w-full rounded-md border transition-all duration-300',
            'border-foreground/20 hover:border-foreground/40',
            'flex h-10 items-center justify-center gap-2',
          )}
        >
          <LuArrowLeft className="text-foreground-alt h-4 w-4" />
          <span className="text-foreground text-sm">Back to pairing</span>
        </button>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col items-center gap-2">
        <div className="bg-brand/10 flex h-10 w-10 items-center justify-center rounded-full">
          <LuShieldCheck className="text-brand h-5 w-5" />
        </div>
        <h2 className="text-foreground text-sm font-medium">
          Verify connection
        </h2>
        <p className="text-foreground-alt text-xs leading-relaxed">
          Do these emoji match what your other device shows?
        </p>
      </div>

      <EmojiGrid emoji={emoji} />

      <div className="flex gap-2">
        <button
          onClick={handleReject}
          className={cn(
            'flex-1 rounded-md border transition-all duration-300',
            'border-destructive/30 hover:bg-destructive/10',
            'flex h-10 items-center justify-center gap-2',
          )}
        >
          <LuX className="text-destructive h-4 w-4" />
          <span className="text-destructive text-sm">No, abort</span>
        </button>
        <button
          onClick={handleConfirm}
          className={cn(
            'flex-1 rounded-md border transition-all duration-300',
            'border-brand/30 bg-brand/10 hover:bg-brand/20',
            'flex h-10 items-center justify-center gap-2',
          )}
        >
          <LuCircleCheck className="text-brand h-4 w-4" />
          <span className="text-foreground text-sm">Yes, they match</span>
        </button>
      </div>
    </div>
  )
}

interface WaitingForConnectionProps {
  onSkip: () => void
  onAbort: () => void
}

function WaitingForConnection({ onSkip, onAbort }: WaitingForConnectionProps) {
  return (
    <div className="space-y-4">
      <div className="flex flex-col items-center gap-2">
        <div className="bg-brand/10 flex h-10 w-10 items-center justify-center rounded-full">
          <LuShieldCheck className="text-brand h-5 w-5" />
        </div>
        <h2 className="text-foreground text-sm font-medium">
          Verify connection
        </h2>
        <p className="text-foreground-alt text-xs leading-relaxed">
          Establishing secure connection with your desktop app...
        </p>
      </div>

      <div className="flex min-h-24 items-center justify-center">
        <Spinner size="lg" className="text-foreground-alt" />
      </div>

      <button
        onClick={onAbort}
        className={cn(
          'w-full rounded-md border transition-all duration-300',
          'border-foreground/20 hover:border-foreground/40',
          'flex h-10 items-center justify-center gap-2',
        )}
      >
        <LuArrowLeft className="text-foreground-alt h-4 w-4" />
        <span className="text-foreground text-sm">Back to pairing</span>
      </button>

      <button
        onClick={onSkip}
        className="text-foreground-alt hover:text-foreground w-full text-center text-xs transition-colors"
      >
        Skip verification for now (not recommended)
      </button>
    </div>
  )
}

function EmojiGrid({ emoji }: { emoji: string[] }) {
  return (
    <div className="grid grid-cols-3 gap-2 px-4">
      {emoji.map((e, i) => (
        <div
          key={i}
          className={cn(
            'border-foreground/10 bg-foreground/5 flex items-center justify-center rounded-lg border',
            'h-16 text-4xl',
          )}
        >
          {e}
        </div>
      ))}
    </div>
  )
}

// -- Direct pairing components (no-cloud WebRTC) --

interface DirectOfferStepProps {
  session: Session | null | undefined
  onRemotePeerResolved: (peerId: string) => void
  onBack: () => void
}

// DirectOfferStep generates a local pairing offer and displays it as a QR
// code and copyable string. After the user pastes back the answerer's payload,
// completes the WebRTC connection and transitions to verification.
function DirectOfferStep({
  session,
  onRemotePeerResolved,
  onBack,
}: DirectOfferStepProps) {
  const [offerPayload, setOfferPayload] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [answerInput, setAnswerInput] = useState('')
  const [accepting, setAccepting] = useState(false)
  const [copied, setCopied] = useState(false)

  // Generate the offer on mount.
  const generateOffer = useCallback(async () => {
    if (!session) return
    setLoading(true)
    setError(null)
    try {
      const resp = await session.createLocalPairingOffer()
      setOfferPayload(resp.offerPayload ?? null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create offer')
    } finally {
      setLoading(false)
    }
  }, [session])

  useEffect(() => {
    generateOffer()
  }, [generateOffer])

  const handleCopy = useCallback(() => {
    if (!offerPayload) return
    navigator.clipboard.writeText(offerPayload)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }, [offerPayload])

  const handleAcceptAnswer = useCallback(async () => {
    if (!session || !answerInput.trim()) return
    setAccepting(true)
    setError(null)
    try {
      const resp = await session.acceptLocalPairingAnswer(answerInput.trim())
      if (resp.remotePeerId) {
        onRemotePeerResolved(resp.remotePeerId)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to accept answer')
    } finally {
      setAccepting(false)
    }
  }, [session, answerInput, onRemotePeerResolved])

  const qrCallback = useCallback(
    (signal: AbortSignal) => {
      if (!offerPayload) return undefined
      const url = `https://spacewave.app/#/pair/${offerPayload}`
      return QRCode.toDataURL(url, {
        width: 200,
        margin: 1,
        color: { dark: '#ffffff', light: '#00000000' },
        errorCorrectionLevel: 'L',
      })
    },
    [offerPayload],
  )
  const qrResult = usePromise(qrCallback)

  return (
    <div className="space-y-4">
      <div className="text-center">
        <div className="mx-auto mb-2 flex h-10 w-10 items-center justify-center">
          <LuWifi className="text-brand h-5 w-5" />
        </div>
        <h2 className="text-foreground text-sm font-medium">
          Direct connection
        </h2>
        <p className="text-foreground-alt mt-1 text-xs leading-relaxed">
          Show this QR code to your other device, or copy the text below.
        </p>
      </div>

      <div className="flex min-h-16 flex-col items-center justify-center gap-3">
        {loading && <Spinner size="lg" className="text-foreground-alt" />}
        {!loading && offerPayload && (
          <>
            {qrResult.data && (
              <img
                src={qrResult.data}
                alt="Direct pairing QR"
                className="h-48 w-48 rounded"
              />
            )}
            <div className="flex w-full items-center gap-2">
              <input
                readOnly
                value={offerPayload}
                className={cn(
                  'border-foreground/20 bg-foreground/5 text-foreground flex-1 rounded-md border px-2 py-1.5 font-mono text-xs',
                  'select-all focus:outline-none',
                )}
                onClick={(e) => (e.target as HTMLInputElement).select()}
              />
              <button
                onClick={handleCopy}
                className={cn(
                  'rounded-md border px-2 py-1.5 transition-all duration-300',
                  'border-foreground/20 hover:border-foreground/40',
                )}
                title="Copy to clipboard"
              >
                <LuCopy
                  className={cn(
                    'h-4 w-4',
                    copied ? 'text-brand' : 'text-foreground-alt',
                  )}
                />
              </button>
            </div>
          </>
        )}
      </div>

      {offerPayload && (
        <div className="space-y-2">
          <p className="text-foreground-alt text-center text-xs">
            Paste the response from the other device:
          </p>
          <textarea
            value={answerInput}
            onChange={(e) => setAnswerInput(e.target.value)}
            placeholder="Paste answer payload here..."
            rows={3}
            className={cn(
              'border-foreground/20 bg-foreground/5 text-foreground w-full resize-none rounded-md border px-2 py-1.5 font-mono text-xs',
              'placeholder:text-foreground/30 focus:border-brand/50 focus:outline-none',
            )}
          />
        </div>
      )}

      {error && <p className="text-destructive text-center text-xs">{error}</p>}

      <div className="flex gap-2">
        <button
          onClick={onBack}
          className={cn(
            'rounded-md border transition-all duration-300',
            'border-foreground/20 hover:border-foreground/40',
            'flex h-10 w-10 shrink-0 items-center justify-center',
          )}
        >
          <LuArrowLeft className="text-foreground-alt h-4 w-4" />
        </button>
        <button
          onClick={handleAcceptAnswer}
          disabled={accepting || !answerInput.trim() || !session}
          className={cn(
            'flex-1 rounded-md border transition-all duration-300',
            'border-brand/30 bg-brand/10 hover:bg-brand/20',
            'disabled:cursor-not-allowed disabled:opacity-50',
            'flex h-10 items-center justify-center gap-2',
          )}
        >
          {accepting ?
            <Spinner />
          : <>
              <LuLink className="text-brand h-4 w-4" />
              <span className="text-foreground text-sm">Connect</span>
            </>
          }
        </button>
      </div>
    </div>
  )
}

interface DirectAnswerStepProps {
  session: Session | null | undefined
  onRemotePeerResolved: (peerId: string) => void
  onBack: () => void
}

// DirectAnswerStep accepts an offer (via QR scan or paste), generates an
// answer, and displays it for the offerer to paste back.
function DirectAnswerStep({
  session,
  onRemotePeerResolved,
  onBack,
}: DirectAnswerStepProps) {
  const [offerInput, setOfferInput] = useState('')
  const [answerPayload, setAnswerPayload] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [scanning, setScanning] = useState(false)
  const [copied, setCopied] = useState(false)

  const handleAcceptOffer = useCallback(
    async (payload: string) => {
      if (!session || !payload.trim()) return
      setLoading(true)
      setError(null)
      try {
        const resp = await session.acceptLocalPairingOffer(payload.trim())
        setAnswerPayload(resp.answerPayload ?? null)
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to accept offer')
      } finally {
        setLoading(false)
      }
    },
    [session],
  )

  const handleSubmit = useCallback(() => {
    handleAcceptOffer(offerInput)
  }, [handleAcceptOffer, offerInput])

  const handleQRScanned = useCallback(
    (decoded: string) => {
      setScanning(false)
      const payload = extractDirectOfferPayload(decoded)
      setOfferInput(payload)
      handleAcceptOffer(payload)
    },
    [handleAcceptOffer],
  )

  const handleCopy = useCallback(() => {
    if (!answerPayload) return
    navigator.clipboard.writeText(answerPayload)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }, [answerPayload])

  // Watch pairing status for peer connection after answer is shared.
  useEffect(() => {
    if (!session || !answerPayload) return
    const controller = new AbortController()
    ;(async () => {
      for await (const resp of session.watchPairingStatus(controller.signal)) {
        if (controller.signal.aborted) break
        if (
          (resp.status === PairingStatus.PairingStatus_PEER_CONNECTED ||
            resp.status === PairingStatus.PairingStatus_VERIFYING_EMOJI) &&
          resp.remotePeerId
        ) {
          onRemotePeerResolved(resp.remotePeerId)
          break
        }
        if (
          resp.status === PairingStatus.PairingStatus_FAILED ||
          resp.status === PairingStatus.PairingStatus_CONNECTION_TIMEOUT
        ) {
          setError(resp.errorMessage || 'Direct connection failed')
          break
        }
      }
    })().catch((err) => {
      if (controller.signal.aborted) return
      setError(err instanceof Error ? err.message : 'Direct connection failed')
    })
    return () => controller.abort()
  }, [session, answerPayload, onRemotePeerResolved])

  return (
    <div className="space-y-4">
      {scanning && (
        <DirectQRScannerModal
          onScanned={handleQRScanned}
          onClose={() => setScanning(false)}
        />
      )}

      <div className="text-center">
        <div className="mx-auto mb-2 flex h-10 w-10 items-center justify-center">
          <LuCamera className="text-brand h-5 w-5" />
        </div>
        <h2 className="text-foreground text-sm font-medium">
          {answerPayload ? 'Share this response' : 'Scan or paste offer'}
        </h2>
        <p className="text-foreground-alt mt-1 text-xs leading-relaxed">
          {answerPayload ?
            'Copy this response and paste it on the other device.'
          : 'Scan the QR code or paste the offer from the other device.'}
        </p>
      </div>

      {!answerPayload && (
        <>
          <textarea
            value={offerInput}
            onChange={(e) => setOfferInput(e.target.value)}
            placeholder="Paste offer payload here..."
            rows={3}
            className={cn(
              'border-foreground/20 bg-foreground/5 text-foreground w-full resize-none rounded-md border px-2 py-1.5 font-mono text-xs',
              'placeholder:text-foreground/30 focus:border-brand/50 focus:outline-none',
            )}
          />

          <button
            onClick={() => setScanning(true)}
            className={cn(
              'w-full rounded-md border transition-all duration-300',
              'border-foreground/10 hover:border-brand/30 hover:bg-brand/5',
              'flex h-9 items-center justify-center gap-2',
            )}
          >
            <LuCamera className="text-foreground-alt h-4 w-4" />
            <span className="text-foreground-alt text-xs">Scan QR code</span>
          </button>
        </>
      )}

      {answerPayload && (
        <div className="space-y-2">
          <div className="flex w-full items-center gap-2">
            <input
              readOnly
              value={answerPayload}
              className={cn(
                'border-foreground/20 bg-foreground/5 text-foreground flex-1 rounded-md border px-2 py-1.5 font-mono text-xs',
                'select-all focus:outline-none',
              )}
              onClick={(e) => (e.target as HTMLInputElement).select()}
            />
            <button
              onClick={handleCopy}
              className={cn(
                'rounded-md border px-2 py-1.5 transition-all duration-300',
                'border-foreground/20 hover:border-foreground/40',
              )}
              title="Copy to clipboard"
            >
              <LuCopy
                className={cn(
                  'h-4 w-4',
                  copied ? 'text-brand' : 'text-foreground-alt',
                )}
              />
            </button>
          </div>
          <div className="flex items-center justify-center gap-2">
            <span className="bg-brand inline-block h-2 w-2 animate-pulse rounded-full" />
            <span className="text-foreground-alt text-xs">
              Waiting for connection...
            </span>
          </div>
        </div>
      )}

      {error && <p className="text-destructive text-center text-xs">{error}</p>}

      <div className="flex gap-2">
        <button
          onClick={onBack}
          className={cn(
            'rounded-md border transition-all duration-300',
            'border-foreground/20 hover:border-foreground/40',
            'flex h-10 w-10 shrink-0 items-center justify-center',
          )}
        >
          <LuArrowLeft className="text-foreground-alt h-4 w-4" />
        </button>
        {!answerPayload && (
          <button
            onClick={handleSubmit}
            disabled={loading || !offerInput.trim() || !session}
            className={cn(
              'flex-1 rounded-md border transition-all duration-300',
              'border-brand/30 bg-brand/10 hover:bg-brand/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
              'flex h-10 items-center justify-center gap-2',
            )}
          >
            {loading ?
              <Spinner />
            : <>
                <LuLink className="text-brand h-4 w-4" />
                <span className="text-foreground text-sm">Accept offer</span>
              </>
            }
          </button>
        )}
      </div>
    </div>
  )
}

// DirectQRScannerModal is a QR scanner for direct pairing payloads (not codes).
function DirectQRScannerModal({
  onScanned,
  onClose,
}: {
  onScanned: (payload: string) => void
  onClose: () => void
}) {
  const scannerRef = useRef<Html5Qrcode | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!containerRef.current) return

    const scanner = new Html5Qrcode(containerRef.current.id)
    scannerRef.current = scanner

    scanner
      .start(
        { facingMode: 'environment' },
        { fps: 10, qrbox: { width: 250, height: 250 } },
        (decoded) => {
          scanner.stop().catch(() => {})
          onScanned(decoded)
        },
        () => {},
      )
      .catch(() => {})

    return () => {
      scanner.stop().catch(() => {})
    }
  }, [onScanned])

  return (
    <div className="bg-background/80 fixed inset-0 z-50 flex items-center justify-center backdrop-blur-sm">
      <div className="border-foreground/20 bg-background w-full max-w-sm rounded-lg border p-4 shadow-xl">
        <div className="mb-3 flex items-center justify-between">
          <h3 className="text-foreground text-sm font-medium">
            Scan direct pairing QR
          </h3>
          <button
            onClick={onClose}
            className="text-foreground-alt hover:text-foreground"
          >
            <LuX className="h-4 w-4" />
          </button>
        </div>
        <div
          id="direct-qr-scanner-container"
          ref={containerRef}
          className="overflow-hidden rounded"
        />
        <p className="text-foreground-alt mt-2 text-center text-xs">
          Point your camera at the QR code on the other device.
        </p>
      </div>
    </div>
  )
}
