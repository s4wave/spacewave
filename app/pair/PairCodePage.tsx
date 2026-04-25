import React, { useCallback, useEffect, useRef, useState } from 'react'
import {
  LuArrowLeft,
  LuCamera,
  LuCircleCheck,
  LuCopy,
  LuKeyboard,
  LuLink,
  LuShieldCheck,
  LuWifi,
  LuX,
} from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'
import { useNavigate, useParams } from '@s4wave/web/router/router.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { createLocalSession } from '@s4wave/app/quickstart/create.js'
import { PairingStatus } from '@s4wave/sdk/session/session.pb.js'
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
  const initialCode =
    isDirectMode ? '' : (
      rawCode
        .replace(/[^A-Za-z0-9]/g, '')
        .toUpperCase()
        .slice(0, 8)
    )
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
  const [sessionIndex, setSessionIndex] = useState<number | null>(null)
  const sessionRef = useRef<Session | null>(providedSession)

  // Keep sessionRef in sync when prop changes.
  useEffect(() => {
    if (!providedSession) return
    sessionRef.current = providedSession
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
        const setup = await createLocalSession(
          root,
          controller.signal,
          registerCleanup,
        )
        session = setup.session
        sessionRef.current = session
        setSessionIndex(setup.sessionIndex)
      }
      const peerId = await session.completePairing(code, controller.signal)
      if (peerId) {
        setRemotePeerId(peerId)
        setStep('verify')
      }
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to complete pairing',
      )
    } finally {
      setLoading(false)
    }
  }, [root, code, registerCleanup])

  const handleBack = useCallback(() => {
    navigate({ path: props.backPath ?? '/' })
  }, [navigate, props.backPath])

  const handleDone = useCallback(() => {
    if (props.donePath) {
      navigate({ path: props.donePath })
    } else if (sessionIndex != null) {
      navigate({ path: `/u/${sessionIndex}` })
    } else {
      navigate({ path: '/' })
    }
  }, [navigate, props.donePath, sessionIndex])

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

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <div className="border-foreground/20 bg-background-get-started w-full max-w-sm rounded-lg border p-6 shadow-lg backdrop-blur-sm">
        <div className="p-0">
          {step === 'enter' && (
            <div className="space-y-4">
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

              <div className="flex justify-center">
                <input
                  type="text"
                  value={formatted}
                  onChange={(e) => handleCodeChange(e.target.value)}
                  onPaste={handlePaste}
                  onKeyDown={handleKeyDown}
                  placeholder="XXXX XXXX"
                  maxLength={9}
                  autoFocus
                  disabled={loading}
                  className={cn(
                    'border-foreground/20 bg-foreground/5 text-foreground w-48 rounded-md border text-center font-mono text-2xl font-bold tracking-[0.2em]',
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
                    'flex h-10 w-10 shrink-0 items-center justify-center',
                  )}
                >
                  <LuArrowLeft className="text-foreground-alt h-4 w-4" />
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
          )}

          {step === 'direct' && (
            <PairDirectStep
              initialOfferPayload={initialOfferPayload}
              session={sessionRef.current}
              root={root ?? null}
              registerCleanup={registerCleanup}
              onSessionCreated={(sess, idx) => {
                sessionRef.current = sess
                setSessionIndex(idx)
              }}
              onRemotePeerResolved={(peerId) => {
                setRemotePeerId(peerId)
                setStep('verify')
              }}
              onBack={handleBack}
            />
          )}

          {step === 'verify' && (
            <PairVerifyStep
              session={sessionRef.current}
              remotePeerId={remotePeerId}
              onConfirmed={() => setStep('done')}
              onAbort={() => {
                setRemotePeerId(null)
                setStep(isDirectMode ? 'direct' : 'enter')
              }}
            />
          )}

          {step === 'done' && (
            <div className="space-y-4">
              <div className="flex flex-col items-center gap-3">
                <div className="bg-brand/10 flex h-12 w-12 items-center justify-center rounded-full">
                  <LuCircleCheck className="text-brand h-6 w-6" />
                </div>
                <h2 className="text-foreground text-sm font-medium">
                  Devices linked!
                </h2>
                <p className="text-foreground-alt text-xs leading-relaxed">
                  Your devices are now connected peer-to-peer.
                </p>
              </div>
              <button
                onClick={handleDone}
                className={cn(
                  'group w-full rounded-md border transition-all duration-300',
                  'border-brand/30 bg-brand/10 hover:bg-brand/20',
                  'flex h-10 items-center justify-center gap-2',
                )}
              >
                <span className="text-foreground text-sm">Go to dashboard</span>
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

interface PairVerifyStepProps {
  session: Session | null
  remotePeerId: string | null
  onConfirmed: () => void
  onAbort: () => void
}

// PairVerifyStep shows SAS emoji verification via WatchPairingStatus and uses
// the bilateral confirmation exchange (confirmSASMatch) for mutual verification.
function PairVerifyStep({
  session,
  remotePeerId,
  onConfirmed,
  onAbort,
}: PairVerifyStepProps) {
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
          if (remotePeerId) {
            await session.confirmPairing(remotePeerId, '', controller.signal)
          }
          if (controller.signal.aborted) break
          onConfirmed()
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
      setError(err instanceof Error ? err.message : 'Pairing failed')
      setWaitingForRemote(false)
    })
    return () => controller.abort()
  }, [session, remotePeerId, onConfirmed])

  const handleConfirm = useCallback(() => {
    if (!session) return
    void session.confirmSASMatch(true)
  }, [session])

  const handleReject = useCallback(() => {
    if (session) {
      void session.confirmSASMatch(false)
    }
    onAbort()
  }, [session, onAbort])

  if (!remotePeerId && !emoji) {
    return (
      <div className="space-y-4 text-center">
        <div className="flex justify-center">
          <Spinner size="md" className="text-foreground-alt" />
        </div>
        <p className="text-foreground-alt text-xs">Waiting for connection...</p>
      </div>
    )
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
          <span className="text-foreground text-sm">Try again</span>
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
        {emoji && (
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
        )}
        <div className="flex min-h-10 items-center justify-center">
          <Spinner size="md" className="text-foreground-alt" />
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
            Setting up encrypted connection...
          </p>
        </div>
        <div className="flex min-h-24 items-center justify-center">
          <Spinner size="md" className="text-foreground-alt" />
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
          <span className="text-foreground text-sm">Back</span>
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

// -- Direct pairing (no-cloud WebRTC answerer) --

interface PairDirectStepProps {
  initialOfferPayload?: string
  session: Session | null
  root: Root | null
  registerCleanup: RegisterCleanup
  onSessionCreated: (session: Session, idx: number) => void
  onRemotePeerResolved: (peerId: string) => void
  onBack: () => void
}

// PairDirectStep handles the no-cloud answerer flow: accepts an offer (via
// paste or QR scan), auto-creates a local session if needed, calls
// acceptLocalPairingOffer, and displays the answer payload for the offerer.
function PairDirectStep({
  initialOfferPayload,
  session,
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
    const setup = await createLocalSession(
      root,
      controller.signal,
      registerCleanup,
    )
    sessionRef.current = setup.session
    onSessionCreated(setup.session, setup.sessionIndex)
    return setup.session
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
          payload.trim(),
          controller.signal,
        )
        setAnswerPayload(resp.answerPayload ?? null)
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to accept offer')
      } finally {
        setLoading(false)
      }
    },
    [ensureSession, registerCleanup],
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
        <div className="mx-auto mb-2 flex h-10 w-10 items-center justify-center">
          <LuWifi className="text-brand h-5 w-5" />
        </div>
        <h2 className="text-foreground text-sm font-medium">
          {answerPayload ? 'Share this response' : 'Direct pairing'}
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
            disabled={loading || !offerInput.trim() || (!session && !root)}
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

// extractDirectOfferPayload extracts the raw b58 offer payload from a QR-decoded
// string. The QR may encode a full URL (https://spacewave.app/#/pair/<payload>)
// or just the raw payload. Returns the payload either way.
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
            <LuX className="h-4 w-4" />
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
