/* eslint-disable react-doctor/rerender-state-only-in-handlers */
import React, { useCallback, useEffect, useRef, useState } from 'react'
import {
  LuArrowLeft,
  LuCamera,
  LuCopy,
  LuLink,
  LuWifi,
  LuX,
} from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'
import { useNavigate, useParams } from '@s4wave/web/router/router.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { preparePairingSession } from './prepare-session.js'
import { resolvedPairingPeer } from './pairing-peer.js'
import { PairingVerificationStep } from '@s4wave/app/session/setup/PairingVerificationStep.js'
import { LinkDeviceDoneStep } from '@s4wave/app/session/setup/LinkDeviceDoneStep.js'
import type { SessionListEntry } from '@s4wave/core/session/session.pb.js'
import { pairingErrorMessage } from '@s4wave/app/session/setup/pairing-copy.js'
import { pairingStatusReachedPeer } from '@s4wave/app/session/pairing-status.js'
import type { Root } from '@s4wave/sdk/root'
import type { Session } from '@s4wave/sdk/session/session.js'
import { Html5Qrcode } from 'html5-qrcode'

type PairStep = 'enter' | 'direct' | 'verify' | 'done'
type DisposableResource = { [Symbol.dispose](): void }

export interface PairCodePageProps {
  // When provided, uses this session instead of creating one on submit.
  session?: Session | null
  // Where the back button navigates. Defaults to '/'.
  backPath?: string
  // Where to navigate after pairing completes. Defaults to '/u/{idx}'.
  donePath?: string
}

function usePairRouteCleanup(): RegisterCleanup {
  const resourcesRef = useRef<DisposableResource[]>([])
  const releasedRef = useRef(false)

  useEffect(() => {
    releasedRef.current = false
    return () => {
      releasedRef.current = true
      for (let i = resourcesRef.current.length - 1; i >= 0; i--) {
        resourcesRef.current[i][Symbol.dispose]()
      }
      resourcesRef.current = []
    }
  }, [])

  return useCallback((resource) => {
    if (!resource) return resource
    if (releasedRef.current) {
      resource[Symbol.dispose]()
      return resource
    }
    resourcesRef.current.push(resource)
    return resource
  }, [])
}

// PairCodePage handles device pairing code entry and verification.
// Two modes: top-level (#/pair/:code) creates a session on submit,
// session-scoped (#/u/N/pair) uses the already-mounted session.
export function PairCodePage(props: PairCodePageProps) {
  const providedSession = props.session ?? null
  const params = useParams()
  const rawCode = params.code ?? ''
  const isDirectMode = rawCode === 'direct' || rawCode.length > 8
  const initialCode = isDirectMode
    ? ''
    : rawCode
        .replace(/[^A-Za-z0-9]/g, '')
        .toUpperCase()
        .slice(0, 8)
  const initialOfferPayload = rawCode.length > 8 ? rawCode : undefined
  const navigate = useNavigate()

  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const registerCleanup = usePairRouteCleanup()

  const [step, setStep] = useState<PairStep>(isDirectMode ? 'direct' : 'enter')
  const [code, setCode] = useState(initialCode)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [remotePeerId, setRemotePeerId] = useState<string | null>(null)
  const [currentSession, setCurrentSession] = useState<Session | null>(
    providedSession,
  )
  const sessionRef = useRef<Session | null>(providedSession)

  // Keep sessionRef in sync when prop changes.
  useEffect(() => {
    if (!providedSession) return
    sessionRef.current = providedSession
    const timer = window.setTimeout(() => setCurrentSession(providedSession), 0)
    return () => window.clearTimeout(timer)
  }, [providedSession])

  const handleCodeChange = useCallback((value: string) => {
    setCode(
      value
        .replace(/[^A-Za-z0-9]/g, '')
        .toUpperCase()
        .slice(0, 8),
    )
  }, [])

  const handlePaste = useCallback((e: React.ClipboardEvent) => {
    e.preventDefault()
    setCode(
      e.clipboardData
        .getData('text')
        .replace(/[^A-Za-z0-9]/g, '')
        .toUpperCase()
        .slice(0, 8),
    )
  }, [])

  const handleSubmit = useCallback(async () => {
    if (code.length < 8) return
    setLoading(true)
    setError(null)
    try {
      let session = sessionRef.current
      const controller = new AbortController()
      registerCleanup({ [Symbol.dispose]: () => controller.abort() })
      if (!session) {
        // No session provided, create one (top-level route).
        if (!root) return
        session = await preparePairingSession(
          root,
          controller.signal,
          registerCleanup,
        )
        sessionRef.current = session
        setCurrentSession(session)
      }
      const peerId = await session.completePairing(
        {
          code,
          offerCurrentAccount: providedSession != null,
          remotePeerId: resolvedPairingPeer(code),
        },
        controller.signal,
      )
      if (peerId) {
        setRemotePeerId(peerId)
        setStep('verify')
      }
    } catch (err) {
      const message =
        err instanceof Error ? err.message : 'Failed to complete pairing'
      setError(pairingErrorMessage(message))
    } finally {
      setLoading(false)
    }
  }, [root, code, registerCleanup, providedSession])

  const handleBack = useCallback(() => {
    navigate({ path: props.backPath ?? '/' })
  }, [navigate, props.backPath])

  const handleDone = useCallback(
    (entry?: SessionListEntry) => {
      navigate({
        path:
          entry?.sessionIndex != null
            ? `/u/${entry.sessionIndex}`
            : (props.donePath ?? '/'),
      })
    },
    [navigate, props.donePath],
  )

  const formatted =
    code.length > 4 ? `${code.slice(0, 4)} ${code.slice(4)}` : code

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (
        e.key === 'Enter' &&
        code.length === 8 &&
        !loading &&
        (providedSession || root)
      ) {
        void handleSubmit()
      }
    },
    [code, loading, providedSession, root, handleSubmit],
  )
  const handleCodeInputRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  return (
    <div className="flex h-full min-h-0 w-full flex-1 items-center justify-center p-4">
      <div className="border-foreground/20 bg-background-get-started w-full max-w-sm rounded-lg border p-6 shadow-lg backdrop-blur-sm">
        <div className="p-0">
          {step === 'enter' && (
            <div className="space-y-4">
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

              <div className="flex justify-center">
                <input
                  ref={handleCodeInputRef}
                  type="text"
                  value={formatted}
                  onChange={(e) => handleCodeChange(e.target.value)}
                  onPaste={handlePaste}
                  onKeyDown={handleKeyDown}
                  placeholder="XXXX XXXX"
                  maxLength={9}
                  disabled={loading}
                  className={cn(
                    'border-foreground/20 bg-foreground/5 text-foreground w-48 rounded-md border text-center font-mono text-2xl font-bold tracking-brand-wide',
                    'placeholder:text-foreground/20 focus:border-brand/50 focus:outline-none',
                    'disabled:cursor-not-allowed disabled:opacity-50',
                    'h-14 px-3',
                  )}
                />
              </div>

              {error && (
                <p className="text-destructive text-center text-xs">{error}</p>
              )}

              <div className="flex gap-2">
                <button
                  onClick={handleBack}
                  className={cn(
                    'rounded-md border transition-all duration-300',
                    'border-foreground/20 hover:border-foreground/40',
                    'flex size-10 shrink-0 items-center justify-center',
                  )}
                >
                  <LuArrowLeft className="text-foreground-alt size-4" />
                </button>
                <button
                  onClick={() => void handleSubmit()}
                  disabled={
                    loading || code.length < 8 || (!providedSession && !root)
                  }
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
              <button
                onClick={() => setStep('direct')}
                disabled={loading}
                className="text-foreground-alt hover:text-foreground w-full text-center text-xs"
              >
                Use a QR code or link
              </button>
            </div>
          )}

          {step === 'direct' && (
            <PairDirectStep
              initialOfferPayload={initialOfferPayload}
              session={currentSession}
              offerCurrentAccount={providedSession != null}
              root={root ?? null}
              registerCleanup={registerCleanup}
              onSessionCreated={(sess) => {
                sessionRef.current = sess
                setCurrentSession(sess)
              }}
              onRemotePeerResolved={(peerId) => {
                setRemotePeerId(peerId)
                setStep('verify')
              }}
              onBack={handleBack}
            />
          )}

          {step === 'verify' && (
            <PairingVerificationStep
              session={currentSession}
              onContinue={() => setStep('done')}
              onAbort={() => {
                setRemotePeerId(null)
                setStep(isDirectMode ? 'direct' : 'enter')
              }}
            />
          )}

          {step === 'done' && (
            <LinkDeviceDoneStep
              session={currentSession}
              remotePeerId={remotePeerId}
              onDone={handleDone}
              onLinkMore={(entry) => {
                if (entry?.sessionIndex != null) {
                  navigate({
                    path: `/u/${entry.sessionIndex}/setup/link-device`,
                  })
                } else {
                  setStep('enter')
                }
              }}
            />
          )}
        </div>
      </div>
    </div>
  )
}

// -- Direct pairing (no-cloud WebRTC answerer) --

interface PairDirectStepProps {
  initialOfferPayload?: string
  session: Session | null
  offerCurrentAccount: boolean
  root: Root | null
  registerCleanup: RegisterCleanup
  onSessionCreated: (session: Session) => void
  onRemotePeerResolved: (peerId: string) => void
  onBack: () => void
}

// PairDirectStep handles the no-cloud answerer flow: accepts an offer (via
// paste or QR scan), auto-creates a local session if needed, calls
// acceptLocalPairingOffer, and displays the answer payload for the offerer.
function PairDirectStep({
  initialOfferPayload,
  session,
  offerCurrentAccount,
  root,
  registerCleanup,
  onSessionCreated,
  onRemotePeerResolved,
  onBack,
}: PairDirectStepProps) {
  const [offerInput, setOfferInput] = useState(initialOfferPayload ?? '')
  const [answerPayload, setAnswerPayload] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [scanning, setScanning] = useState(false)
  const [copied, setCopied] = useState(false)
  const sessionRef = useRef<Session | null>(session)

  useEffect(() => {
    if (!session) return
    sessionRef.current = session
  }, [session])

  const ensureSession = useCallback(async (): Promise<Session | null> => {
    if (sessionRef.current) return sessionRef.current
    if (!root) return null
    const controller = new AbortController()
    registerCleanup({ [Symbol.dispose]: () => controller.abort() })
    const session = await preparePairingSession(
      root,
      controller.signal,
      registerCleanup,
    )
    sessionRef.current = session
    onSessionCreated(session)
    return session
  }, [root, registerCleanup, onSessionCreated])

  const handleAcceptOffer = useCallback(
    async (payload: string) => {
      if (!payload.trim()) return
      setLoading(true)
      setError(null)
      try {
        const sess = await ensureSession()
        if (!sess) return
        const controller = new AbortController()
        registerCleanup({ [Symbol.dispose]: () => controller.abort() })
        const resp = await sess.acceptLocalPairingOffer(
          extractDirectOfferPayload(payload.trim()),
          offerCurrentAccount,
          controller.signal,
        )
        setAnswerPayload(resp.answerPayload ?? null)
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to accept offer')
      } finally {
        setLoading(false)
      }
    },
    [ensureSession, registerCleanup, offerCurrentAccount],
  )

  // Auto-accept if initial offer payload was provided via URL.
  const autoAccepted = useRef(false)
  useEffect(() => {
    if (initialOfferPayload && !autoAccepted.current) {
      autoAccepted.current = true
      void handleAcceptOffer(initialOfferPayload)
    }
  }, [initialOfferPayload, handleAcceptOffer])

  const handleSubmit = useCallback(() => {
    void handleAcceptOffer(offerInput)
  }, [handleAcceptOffer, offerInput])

  const handleQRScanned = useCallback(
    (decoded: string) => {
      setScanning(false)
      const payload = extractDirectOfferPayload(decoded)
      setOfferInput(payload)
      void handleAcceptOffer(payload)
    },
    [handleAcceptOffer],
  )

  const handleCopy = useCallback(() => {
    if (!answerPayload) return
    void navigator.clipboard.writeText(answerPayload)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }, [answerPayload])

  // Watch pairing status for peer connection after answer is shared.
  useEffect(() => {
    const sess = sessionRef.current
    if (!sess || !answerPayload) return
    const controller = new AbortController()
    ;(async () => {
      for await (const resp of sess.watchPairingStatus(controller.signal)) {
        if (controller.signal.aborted) break
        if (pairingStatusReachedPeer(resp.status) && resp.remotePeerId) {
          onRemotePeerResolved(resp.remotePeerId)
          break
        }
      }
    })().catch((err) => {
      if (controller.signal.aborted) return
      setError(err instanceof Error ? err.message : 'Direct connection failed')
    })
    return () => controller.abort()
  }, [answerPayload, onRemotePeerResolved])

  return (
    <div className="space-y-4">
      {scanning && (
        <PairDirectQRScanner
          onScanned={handleQRScanned}
          onClose={() => setScanning(false)}
        />
      )}

      <div className="text-center">
        <div className="mx-auto mb-2 flex size-10 items-center justify-center">
          <LuWifi className="text-brand size-5" />
        </div>
        <h2 className="text-foreground text-sm font-medium">
          {answerPayload ? 'Share this response' : 'Direct pairing'}
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
            placeholder="Paste offer payload here…"
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
            <LuCamera className="text-foreground-alt size-4" />
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
                  'size-4',
                  copied ? 'text-brand' : 'text-foreground-alt',
                )}
              />
            </button>
          </div>
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
        <button
          onClick={onBack}
          className={cn(
            'rounded-md border transition-all duration-300',
            'border-foreground/20 hover:border-foreground/40',
            'flex size-10 shrink-0 items-center justify-center',
          )}
        >
          <LuArrowLeft className="text-foreground-alt size-4" />
        </button>
        {!answerPayload && (
          <button
            onClick={handleSubmit}
            disabled={loading || !offerInput.trim() || (!session && !root)}
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

// extractDirectOfferPayload accepts a pairing URL or its encoded offer.
function extractDirectOfferPayload(decoded: string): string {
  const match = decoded.match(/#\/pair\/(.+)/)
  if (match) return match[1]
  return decoded
}

// PairDirectQRScanner renders a camera-based QR scanner for direct pairing payloads.
function PairDirectQRScanner({
  onScanned,
  onClose,
}: {
  onScanned: (payload: string) => void
  onClose: () => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!containerRef.current) return
    const scanner = new Html5Qrcode(containerRef.current.id)
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
            <LuX className="size-4" />
          </button>
        </div>
        <div
          id="pair-direct-qr-scanner"
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
