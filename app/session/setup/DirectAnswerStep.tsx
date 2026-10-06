import { useState } from 'react'
import { LuCamera, LuLink } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import type { Session } from '@s4wave/sdk/session/session.js'

import { CopyablePayloadField } from './CopyablePayloadField.js'
import { LinkDeviceBackButton } from './LinkDeviceBackButton.js'
import { QRScannerModal } from './QRScannerModal.js'
import { usePairingWatch } from './usePairingWatch.js'

// extractDirectOfferPayload extracts the raw b58 offer payload from a QR-decoded
// string. The QR may encode a full URL (https://spacewave.app/#/pair/<payload>)
// or just the raw payload. Returns the payload either way.
function extractDirectOfferPayload(decoded: string): string {
  const match = decoded.match(/#\/pair\/(.+)/)
  if (match) return match[1]
  return decoded
}

export interface DirectAnswerStepProps {
  session: Session | null | undefined
  onRemotePeerResolved: (peerId: string) => void
  onBack: () => void
}

// DirectAnswerStep accepts an offer (via QR scan or paste), generates an
// answer, and displays it for the offerer to paste back.
export function DirectAnswerStep({
  session,
  onRemotePeerResolved,
  onBack,
}: DirectAnswerStepProps) {
  const [offerInput, setOfferInput] = useState('')
  const [answerPayload, setAnswerPayload] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [scanning, setScanning] = useState(false)

  const handleAcceptOffer = async (payload: string) => {
    if (!session || !payload.trim()) return
    setLoading(true)
    setError(null)
    try {
      const resp = await session.acceptLocalPairingOffer(payload.trim(), true)
      setAnswerPayload(resp.answerPayload ?? null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to accept offer')
    } finally {
      setLoading(false)
    }
  }

  const handleDecoded = (decoded: string) => {
    setScanning(false)
    const payload = extractDirectOfferPayload(decoded)
    setOfferInput(payload)
    void handleAcceptOffer(payload)
    return true
  }

  // Watch pairing status for peer connection after answer is shared.
  usePairingWatch({
    session,
    enabled: !!answerPayload,
    reportFailures: false,
    streamFailure: 'Direct connection failed',
    onPeerResolved: onRemotePeerResolved,
    onFailure: setError,
  })

  return (
    <div className="space-y-4">
      {scanning && (
        <QRScannerModal
          title="Scan direct pairing QR"
          hint="Point your camera at the QR code on the other device."
          containerId="direct-qr-scanner-container"
          qrboxSize={250}
          onDecoded={handleDecoded}
          onClose={() => setScanning(false)}
        />
      )}

      <div className="text-center">
        <div className="mx-auto mb-2 flex size-10 items-center justify-center">
          <LuCamera className="text-brand size-5" />
        </div>
        <h2 className="text-foreground text-sm font-medium">
          {answerPayload ? 'Share this response' : 'Scan or paste offer'}
        </h2>
        <p className="text-foreground-alt mt-1 text-xs leading-relaxed">
          {answerPayload
            ? 'Copy this response and paste it on the other device.'
            : 'Scan the QR code or paste the offer from the other device.'}
        </p>
      </div>

      {!answerPayload && (
        <>
          <textarea
            value={offerInput}
            onChange={(e) => setOfferInput(e.target.value)}
            aria-label="Offer payload"
            placeholder="Paste offer payload here…"
            rows={3}
            className={cn(
              'border-foreground/20 bg-foreground/5 text-foreground w-full resize-none rounded-md border px-2 py-1.5 font-mono text-xs',
              'placeholder:text-foreground/30 focus:border-brand/50 focus:outline-none',
            )}
          />

          <button
            type="button"
            onClick={() => setScanning(true)}
            className={cn(
              'w-full rounded-md border transition-all duration-300',
              'border-foreground/10 hover:border-brand/30 hover:bg-brand/5',
              'flex h-9 items-center justify-center gap-2',
            )}
          >
            <LuCamera className="text-foreground-alt size-4" />
            <span className="text-foreground-alt text-xs">Scan QR code</span>
          </button>
        </>
      )}

      {answerPayload && (
        <div className="space-y-2">
          <CopyablePayloadField value={answerPayload} label="Answer payload" />
          <div className="flex items-center justify-center gap-2">
            <span className="bg-brand inline-block size-2 animate-pulse rounded-full" />
            <span className="text-foreground-alt text-xs">
              Waiting for connection…
            </span>
          </div>
        </div>
      )}

      {error && <p className="text-destructive text-center text-xs">{error}</p>}

      <div className="flex gap-2">
        <LinkDeviceBackButton onClick={onBack} />
        {!answerPayload && (
          <button
            type="button"
            onClick={() => void handleAcceptOffer(offerInput)}
            disabled={loading || !offerInput.trim() || !session}
            className={cn(
              'flex-1 rounded-md border transition-all duration-300',
              'border-brand/30 bg-brand/10 hover:bg-brand/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
              'flex h-10 items-center justify-center gap-2',
            )}
          >
            {loading ? (
              <Spinner />
            ) : (
              <>
                <LuLink className="text-brand size-4" />
                <span className="text-foreground text-sm">Accept offer</span>
              </>
            )}
          </button>
        )}
      </div>
    </div>
  )
}
