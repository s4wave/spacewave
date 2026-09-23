import '../../sdk/dispose-symbol.js'

import {
  Session,
  SendRequest,
  WatchRequest,
} from '../../frontend/frontend.pb.js'
import type { Frontend } from '../../frontend/frontend_srpc.pb.js'
import { retryWithAbort } from './retry.js'

/** FrontendHandlers is Vite's module runner transport callback boundary. */
interface FrontendHandlers {
  onMessage(payload: unknown): void
}

declare global {
  var __bldrFrontendEnabled: boolean | undefined
  var __bldrFrontends: Map<string, FrontendResource> | undefined
}

/** FrontendResource retains one ordered compiler subscription for its owner. */
export class FrontendResource {
  private readonly abort = new AbortController()
  private readonly ready: Promise<Session>
  private resolveReady!: (session: Session) => void
  private rejectReady!: (reason: unknown) => void
  private session?: Session
  private clientReady?: Promise<unknown>
  private sequence = 0n
  private handlers?: FrontendHandlers
  private readonly pending: unknown[] = []

  constructor(
    private readonly client: Pick<Frontend, 'Watch' | 'Send'>,
    private readonly onInvalidated: (error: Error) => void = () =>
      location.reload(),
    private readonly reconnect = true,
  ) {
    // A compiler snapshot admits this connection before its modules can load.
    this.ready = new Promise((resolve, reject) => {
      this.resolveReady = resolve
      this.rejectReady = reject
    })
    void this.ready.catch(() => {})

    // Reconnection is safe only while the compiler and sequence are unchanged.
    void retryWithAbort(
      this.abort.signal,
      async (signal) => {
        let snapshot = true
        for await (const event of this.client.Watch(
          WatchRequest.create(),
          signal,
        )) {
          signal.throwIfAborted()
          const sequence = event.sequence ?? 0n
          if (snapshot) {
            snapshot = false
            const session = event.session
            if (
              !session?.id ||
              !/^\/b\/fe\/(?:rpc\/[a-zA-Z0-9_-]+\/)?[a-zA-Z0-9-]+\/$/.test(
                session.routePrefix ?? '',
              ) ||
              !session.routePrefix?.endsWith(`/${session.id}/`)
            ) {
              throw new Error('Bldr frontend returned an invalid session')
            }
            if (
              this.session &&
              (this.session.id !== session.id || this.sequence !== sequence)
            ) {
              this.invalidate('Bldr frontend session changed')
              return
            }
            const frontends = (globalThis.__bldrFrontends ??= new Map())
            const existing = frontends.get(session.id)
            if (existing && existing !== this) {
              this.invalidate('Bldr frontend session is already attached')
              return
            }
            this.session = session
            this.sequence = sequence
            frontends.set(session.id, this)
            this.resolveReady(session)
            continue
          }
          if (sequence !== this.sequence + 1n) {
            this.invalidate('Bldr frontend update history has a gap')
            return
          }
          this.sequence = sequence
          if (event.payload) this.receive(JSON.parse(event.payload))
        }
        throw new Error('Bldr frontend update stream ended')
      },
      {
        errorCb: (error) => {
          if (!this.reconnect) {
            this.invalidate(
              error instanceof Error ? error.message : String(error),
            )
            return
          }
          console.debug('Bldr frontend reconnecting', error)
        },
      },
    ).catch((error) => {
      if (!this.abort.signal.aborted) this.rejectReady(error)
    })
  }

  /** resolve starts the compiler client before loading an admitted entrypoint. */
  public async resolve(entrypoint: string): Promise<string> {
    const session = await this.getSession()
    if (!session.entrypoints?.includes(entrypoint)) {
      throw new Error(
        `Bldr frontend entrypoint is not configured: ${entrypoint}`,
      )
    }
    // Vite can replace dependencies while the first application graph loads.
    // Start its client independently so a failed import cannot strand a reload.
    const clientURL = session.routePrefix + '@vite/client'
    this.clientReady ??= import(/* @vite-ignore */ clientURL)
    await this.clientReady
    this.abort.signal.throwIfAborted()
    return session.routePrefix + entrypoint
  }

  /** getSession waits for admission and rejects access after owner release. */
  public async getSession(): Promise<Session> {
    const session = await this.ready
    this.abort.signal.throwIfAborted()
    return session
  }

  /** connect attaches the upstream client after the initial session snapshot. */
  public async connect(handlers: FrontendHandlers): Promise<void> {
    await this.getSession()
    this.handlers = handlers
    handlers.onMessage({ type: 'connected' })
    for (const payload of this.pending.splice(0)) handlers.onMessage(payload)
  }

  /** send forwards upstream custom events through the existing RPC client. */
  public async send(payload: unknown): Promise<void> {
    const session = await this.getSession()
    await this.client.Send(
      SendRequest.create({
        sessionId: session.id,
        payload: JSON.stringify(payload),
      }),
      this.abort.signal,
    )
  }

  /** disconnect detaches Vite; its owner still retains the subscription. */
  public disconnect(): void {
    this.handlers = undefined
  }

  /** release cancels the stream and any delayed reconnect. */
  public release(): void {
    const error = new Error('Bldr frontend attachment closed')
    this.abort.abort(error)
    this.rejectReady(error)
    this.handlers = undefined
    this.pending.length = 0
    if (
      this.session?.id &&
      globalThis.__bldrFrontends?.get(this.session.id) === this
    ) {
      globalThis.__bldrFrontends.delete(this.session.id)
    }
  }

  /** [Symbol.dispose] releases the compiler subscription with its owner. */
  public [Symbol.dispose](): void {
    this.release()
  }

  private receive(payload: unknown): void {
    // A retained authoring view replaces its own compiler after invalidation.
    // Vite must not reload every other open app in the containing document.
    if (
      !this.reconnect &&
      payload !== null &&
      typeof payload === 'object' &&
      'type' in payload &&
      payload.type === 'full-reload'
    ) {
      this.invalidate('Frontend configuration changed. Restart the preview.')
      return
    }

    if (this.handlers) this.handlers.onMessage(payload)
    else if (this.pending.length < 64) this.pending.push(payload)
    else this.invalidate('Bldr frontend update consumer fell behind')
  }

  private invalidate(message: string): void {
    if (this.abort.signal.aborted) return
    this.release()
    this.onInvalidated(new Error(message))
  }
}
