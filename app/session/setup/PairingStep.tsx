import { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react'
import { LuLink, LuRefreshCw } from 'react-icons/lu'
import QRCode from 'qrcode'

import { cn } from '@s4wave/web/style/utils.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { SPACEWAVE_PUBLIC_BASE_URL } from '@s4wave/app/urls.js'
import type { Session } from '@s4wave/sdk/session/session.js'

import { LinkDeviceBackButton } from './LinkDeviceBackButton.js'
import { PairingCodeChip } from './PairingCodeChip.js'
import {
  PAIRING_CODE_INSTRUCTIONS,
  pairingErrorMessage,
} from './pairing-copy.js'
import { usePairingWatch } from './usePairingWatch.js'

const CODE_TTL_SECONDS = 600

// PairingQRCode renders a QR code encoding the deep link URL for the pairing code.
function PairingQRCode({ code }: { code: string }) {
  const qrCallback = useCallback(() => {
    if (!code) return undefined
    const url = `${SPACEWAVE_PUBLIC_BASE_URL}/#/pair/${code}`
    return QRCode.toDataURL(url, {
      width: 160,
      margin: 1,
      color: { dark: '#ffffff', light: '#00000000' },
    })
  }, [code])
  const qrResult = usePromise(qrCallback)

  if (!qrResult.data) return null

  return (
    <div className="flex flex-col items-center gap-1">
      <img
        src={qrResult.data}
        alt="Pairing QR code"
        className="size-40 rounded"
      />
      <span className="text-foreground-alt text-xs">Or scan this QR code</span>
    </div>
  )
}

// useCodeCountdown counts down the pairing code lifetime from the moment the
// code arrives, and calls onExpired once when it reaches zero.
function useCodeCountdown(code: string | null, onExpired: () => void) {
  const [elapsed, setElapsed] = useState(0)

  useEffect(() => {
    if (!code) return
    const interval = setInterval(() => {
      setElapsed((e) => e + 1)
    }, 1000)
    return () => clearInterval(interval)
  }, [code])

  const secondsLeft = code
    ? Math.max(0, CODE_TTL_SECONDS - elapsed)
    : CODE_TTL_SECONDS

  const handleExpired = useEffectEvent(onExpired)
  const prevSecondsLeft = useRef(secondsLeft)
  useEffect(() => {
    if (prevSecondsLeft.current > 0 && secondsLeft === 0 && code) {
      handleExpired()
    }
    prevSecondsLeft.current = secondsLeft
  }, [secondsLeft, code])

  const mins = Math.floor(secondsLeft / 60)
  const secs = secondsLeft % 60
  return {
    secondsLeft,
    countdown: `${mins}:${secs.toString().padStart(2, '0')}`,
  }
}

export interface PairingStepProps {
  session: Session | null | undefined
  onRemotePeerResolved: (peerId: string) => void
  onBack: () => void
}

// PairingStep shows a pairing code for the other device to enter. Generating a
// new code remounts the code view, so each code starts with a fresh countdown
// and connection state.
export function PairingStep({
  session,
  onRemotePeerResolved,
  onBack,
}: PairingStepProps) {
  const [generation, setGeneration] = useState(0)

  return (
    <PairingCodeView
      key={generation}
      session={session}
      onRegenerateCode={() => setGeneration((n) => n + 1)}
      onRemotePeerResolved={onRemotePeerResolved}
      onBack={onBack}
    />
  )
}

interface PairingCodeViewProps extends PairingStepProps {
  onRegenerateCode: () => void
}

// PairingCodeView generates one pairing code and watches for the remote device
// to connect with it.
function PairingCodeView({
  session,
  onRegenerateCode,
  onRemotePeerResolved,
  onBack,
}: PairingCodeViewProps) {
  const generateCode = useCallback(
    (signal: AbortSignal) => session?.generatePairingCode(signal),
    [session],
  )
  const codeResult = usePromise(generateCode)
  const code = codeResult.data ?? null
  const loading = codeResult.loading
  const [connectionError, setConnectionError] = useState<string | null>(null)

  usePairingWatch({
    session,
    enabled: !!code,
    reportFailures: true,
    streamFailure: 'Pairing status stream failed',
    onPeerResolved: onRemotePeerResolved,
    onFailure: setConnectionError,
  })
  const { secondsLeft, countdown } = useCodeCountdown(code, onRegenerateCode)

  const instructions = PAIRING_CODE_INSTRUCTIONS

  return (
    <div className="border-foreground/20 bg-background-get-started w-full rounded-lg border p-6 shadow-lg backdrop-blur-sm">
      <div className="text-center">
        <div className="mx-auto mb-2 flex size-10 items-center justify-center">
          <LuLink className="text-brand size-5" />
        </div>
        <h2 className="text-foreground text-sm font-medium">Link My Device</h2>
        <p className="text-foreground-alt mt-1 text-xs">
          {instructions.heading}
        </p>
        <p className="text-foreground-alt mt-1 text-xs">{instructions.hint}</p>
      </div>

      <div className="flex min-h-16 flex-col items-center justify-center gap-3">
        {loading && <Spinner size="lg" variant="muted" />}
        {!loading && code && (
          <>
            <PairingCodeChip code={code} />
            <PairingQRCode code={code} />
          </>
        )}
      </div>

      {connectionError && (
        <div className="flex flex-col items-center gap-2">
          <p className="text-destructive text-center text-xs">
            {connectionError}
          </p>
          <button
            type="button"
            onClick={onRegenerateCode}
            className={cn(
              'rounded-md border px-3 py-1.5 text-xs transition-all duration-300',
              'border-foreground/20 hover:border-foreground/40',
            )}
          >
            Retry
          </button>
        </div>
      )}

      {codeResult.error && !connectionError && (
        <p className="text-destructive text-center text-xs">
          {pairingErrorMessage(codeResult.error.message)}
        </p>
      )}

      <div className="flex gap-2">
        <LinkDeviceBackButton onClick={onBack} />
        <button
          type="button"
          onClick={onRegenerateCode}
          disabled={loading}
          className={cn(
            'flex-1 rounded-md border transition-all duration-300',
            'border-foreground/20 hover:border-foreground/40',
            'disabled:cursor-not-allowed disabled:opacity-50',
            'flex h-10 items-center justify-center gap-2',
          )}
        >
          <LuRefreshCw className="text-foreground-alt size-4" />
          <span className="text-foreground text-sm">Generate new code</span>
        </button>
      </div>
      {!loading && code && !connectionError && (
        <WaitingForConnection secondsLeft={secondsLeft} countdown={countdown} />
      )}
    </div>
  )
}

// WaitingForConnection shows the pending connection and the code expiry.
function WaitingForConnection({
  secondsLeft,
  countdown,
}: {
  secondsLeft: number
  countdown: string
}) {
  return (
    <div className="flex flex-col items-center gap-1">
      <div className="flex items-center justify-center gap-2">
        <span className="bg-brand inline-block size-2 animate-pulse rounded-full" />
        <span className="text-foreground-alt text-xs">
          Waiting for connection…
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
  )
}
