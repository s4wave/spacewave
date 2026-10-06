import { useState } from 'react'
import { LuCamera, LuLink } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import type { Session } from '@s4wave/sdk/session/session.js'

import { LinkDeviceBackButton } from './LinkDeviceBackButton.js'
import { pairingErrorMessage } from './pairing-copy.js'
import { QRScannerModal } from './QRScannerModal.js'

// extractCodeFromQR extracts a pairing code from a QR-encoded URL or raw code string.
function extractCodeFromQR(decoded: string): string | null {
  const urlMatch = decoded.match(/#\/pair\/([A-Za-z0-9]{8})/)
  if (urlMatch) return urlMatch[1].toUpperCase()
  const cleaned = decoded.replace(/[^A-Za-z0-9]/g, '')
  if (cleaned.length === 8) return cleaned.toUpperCase()
  return null
}

// cleanCode keeps the alphanumeric characters of a typed or pasted pairing
// code, upper-cased and limited to the 8-character code length.
function cleanCode(value: string): string {
  return value
    .replace(/[^A-Za-z0-9]/g, '')
    .toUpperCase()
    .slice(0, 8)
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

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && value.length === 8 && !disabled) {
      onSubmit?.()
    }
  }
  const handleInputRef = (node: HTMLInputElement | null) => {
    node?.focus()
  }

  return (
    <div className="flex justify-center">
      <input
        ref={handleInputRef}
        type="text"
        value={formatted}
        onChange={(e) => onChange(e.target.value)}
        onPaste={onPaste}
        onKeyDown={handleKeyDown}
        aria-label="Pairing code"
        placeholder="XXXX XXXX"
        maxLength={9}
        disabled={disabled}
        className={cn(
          'border-foreground/20 bg-foreground/5 text-foreground w-48 rounded-md border text-center font-mono text-2xl font-bold tracking-brand-wide',
          'placeholder:text-foreground/20 focus:border-brand/50 focus:outline-none',
          'disabled:cursor-not-allowed disabled:opacity-50',
          'h-14 px-3',
        )}
      />
    </div>
  )
}

export interface EnterCodeStepProps {
  session: Session | null | undefined
  onRemotePeerResolved: (peerId: string) => void
  onBack: () => void
}

// EnterCodeStep completes pairing with a code typed, pasted, or scanned from
// the other device.
export function EnterCodeStep({
  session,
  onRemotePeerResolved,
  onBack,
}: EnterCodeStepProps) {
  const [code, setCode] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [scanning, setScanning] = useState(false)

  const handleSubmit = async () => {
    if (!session || code.length < 8) return
    setLoading(true)
    setError(null)
    try {
      const remotePeerId = await session.completePairing({
        code: code.replace(/\s/g, ''),
        offerCurrentAccount: true,
      })
      if (remotePeerId) {
        onRemotePeerResolved(remotePeerId)
      }
    } catch (err) {
      const message =
        err instanceof Error ? err.message : 'Failed to complete pairing'
      setError(pairingErrorMessage(message))
      // Surface via toast too: a failed submit can re-render or navigate the
      // settings pane away from this card before the inline error is seen.
      toast.error(pairingErrorMessage(message))
    } finally {
      setLoading(false)
    }
  }

  const handlePaste = (e: React.ClipboardEvent) => {
    e.preventDefault()
    setCode(cleanCode(e.clipboardData.getData('text')))
  }

  const handleDecoded = (decoded: string) => {
    const scanned = extractCodeFromQR(decoded)
    if (!scanned) return false
    setCode(scanned)
    setScanning(false)
    return true
  }

  return (
    <div className="space-y-4">
      {scanning && (
        <QRScannerModal
          title="Scan QR code"
          hint="Point your camera at the QR code on your other device."
          containerId="qr-scanner-container"
          qrboxSize={200}
          onDecoded={handleDecoded}
          onClose={() => setScanning(false)}
        />
      )}

      <div className="text-center">
        <div className="mx-auto mb-2 flex size-10 items-center justify-center">
          <LuLink className="text-brand size-5" />
        </div>
        <h2 className="text-foreground text-sm font-medium">
          Pair another device
        </h2>
        <p className="text-foreground-alt mt-1 text-xs leading-relaxed">
          Enter the 8-character code shown on your other device.
        </p>
      </div>

      <CodeInput
        value={code}
        onChange={(value) => setCode(cleanCode(value))}
        onPaste={handlePaste}
        onSubmit={() => {
          void handleSubmit()
        }}
        disabled={loading}
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

      {error && <p className="text-destructive text-center text-xs">{error}</p>}

      <div className="flex gap-2">
        <LinkDeviceBackButton onClick={onBack} />
        <button
          type="button"
          onClick={() => {
            void handleSubmit()
          }}
          disabled={loading || code.length < 8 || !session}
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
              <span className="text-foreground text-sm">Connect</span>
            </>
          )}
        </button>
      </div>
    </div>
  )
}
