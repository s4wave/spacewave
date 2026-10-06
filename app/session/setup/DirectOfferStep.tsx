import { useCallback, useEffect, useState } from 'react'
import { LuLink, LuWifi } from 'react-icons/lu'
import QRCode from 'qrcode'

import { cn } from '@s4wave/web/style/utils.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { SPACEWAVE_PUBLIC_BASE_URL } from '@s4wave/app/urls.js'
import type { Session } from '@s4wave/sdk/session/session.js'

import { CopyablePayloadField } from './CopyablePayloadField.js'
import { LinkDeviceBackButton } from './LinkDeviceBackButton.js'

export interface DirectOfferStepProps {
  session: Session | null | undefined
  onRemotePeerResolved: (peerId: string) => void
  onBack: () => void
}

// DirectOfferStep generates a local pairing offer and displays it as a QR
// code and copyable string. After the user pastes back the answerer's payload,
// completes the WebRTC connection and transitions to verification.
export function DirectOfferStep({
  session,
  onRemotePeerResolved,
  onBack,
}: DirectOfferStepProps) {
  const [offerPayload, setOfferPayload] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [answerInput, setAnswerInput] = useState('')
  const [accepting, setAccepting] = useState(false)

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
    queueMicrotask(() => {
      void generateOffer()
    })
  }, [generateOffer])

  const handleAcceptAnswer = async () => {
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
  }

  const qrCallback = useCallback(() => {
    if (!offerPayload) return undefined
    const url = `${SPACEWAVE_PUBLIC_BASE_URL}/#/pair/${offerPayload}`
    return QRCode.toDataURL(url, {
      width: 200,
      margin: 1,
      color: { dark: '#ffffff', light: '#00000000' },
      errorCorrectionLevel: 'L',
    })
  }, [offerPayload])
  const qrResult = usePromise(qrCallback)

  return (
    <div className="space-y-4">
      <div className="text-center">
        <div className="mx-auto mb-2 flex size-10 items-center justify-center">
          <LuWifi className="text-brand size-5" />
        </div>
        <h2 className="text-foreground text-sm font-medium">
          Direct connection
        </h2>
        <p className="text-foreground-alt mt-1 text-xs leading-relaxed">
          Show this QR code to your other device, or copy the text below.
        </p>
      </div>

      <div className="flex min-h-16 flex-col items-center justify-center gap-3">
        {loading && <Spinner size="lg" variant="muted" />}
        {!loading && offerPayload && (
          <>
            {qrResult.data && (
              <img
                src={qrResult.data}
                alt="Direct pairing QR"
                className="size-48 rounded"
              />
            )}
            <CopyablePayloadField value={offerPayload} label="Offer payload" />
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
            aria-label="Answer payload"
            placeholder="Paste answer payload here…"
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
        <LinkDeviceBackButton onClick={onBack} />
        <button
          type="button"
          onClick={() => {
            void handleAcceptAnswer()
          }}
          disabled={accepting || !answerInput.trim() || !session}
          className={cn(
            'flex-1 rounded-md border transition-all duration-300',
            'border-brand/30 bg-brand/10 hover:bg-brand/20',
            'disabled:cursor-not-allowed disabled:opacity-50',
            'flex h-10 items-center justify-center gap-2',
          )}
        >
          {accepting ? (
            <Spinner />
          ) : (
            <>
              <LuLink className="text-brand size-4" />
              <span className="text-foreground text-sm">Connect</span>
            </>
          )}
        </button>
      </div>
    </div>
  )
}
