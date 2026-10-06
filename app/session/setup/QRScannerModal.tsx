import { useEffect, useEffectEvent, useRef } from 'react'
import { Html5Qrcode } from 'html5-qrcode'
import { LuX } from 'react-icons/lu'

export interface QRScannerModalProps {
  title: string
  hint: string
  // containerId names the element html5-qrcode renders the camera into.
  containerId: string
  qrboxSize: number
  // onDecoded receives each decoded QR string. It returns true once it accepts
  // the payload, which stops the scanner.
  onDecoded: (decoded: string) => boolean
  onClose: () => void
}

// QRScannerModal renders a camera-based QR scanner overlay using html5-qrcode.
export function QRScannerModal({
  title,
  hint,
  containerId,
  qrboxSize,
  onDecoded,
  onClose,
}: QRScannerModalProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const handleDecoded = useEffectEvent(onDecoded)

  useEffect(() => {
    if (!containerRef.current) return

    const scanner = new Html5Qrcode(containerRef.current.id)
    scanner
      .start(
        { facingMode: 'environment' },
        { fps: 10, qrbox: { width: qrboxSize, height: qrboxSize } },
        (decoded) => {
          if (handleDecoded(decoded)) {
            scanner.stop().catch(() => {})
          }
        },
        () => {},
      )
      .catch(() => {})

    return () => {
      scanner.stop().catch(() => {})
    }
  }, [qrboxSize])

  return (
    <div className="bg-background/80 fixed inset-0 z-50 flex items-center justify-center backdrop-blur-sm">
      <div className="border-foreground/20 bg-background w-full max-w-sm rounded-lg border p-4 shadow-xl">
        <div className="mb-3 flex items-center justify-between">
          <h3 className="text-foreground text-sm font-medium">{title}</h3>
          <button
            type="button"
            aria-label="Close"
            onClick={onClose}
            className="text-foreground-alt hover:text-foreground"
          >
            <LuX className="size-4" />
          </button>
        </div>
        <div
          id={containerId}
          ref={containerRef}
          className="overflow-hidden rounded"
        />
        <p className="text-foreground-alt mt-2 text-center text-xs">{hint}</p>
      </div>
    </div>
  )
}
