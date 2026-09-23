import type { Engine } from '../../sdk/world/engine.js'
import type { AppSource } from '../../sdk/sync/app.js'
import type { Server as HTTPServer } from 'node:http'
import { createAccess, type DatabaseAccess } from '../../sdk/sync/access.js'
import { publicError, SyncError } from '../../sdk/sync/errors.js'
import type { Principal, Schema } from '../../sdk/sync/schema.js'
import {
  Application,
  type ApplicationOptions,
  type Migration,
} from './application.js'
import { openNodeEngine, type OwnedEngine } from './node/host.js'
import {
  attachListener,
  listen,
  type Attachment,
  type AttachmentOptions,
  type Listener,
  type ListenerOptions,
} from './listener.js'

export type ServerOptions<S extends Schema, P extends Principal> = Omit<
  ApplicationOptions<S, P>,
  'engine'
> &
  (
    | { directory: string; engine?: never }
    | { engine: Engine; directory?: never }
  ) & {
    authenticate(token: string, signal: AbortSignal): Promise<P>
    migration?: Migration<S>
  }

// SyncServer owns admission and any engine it opened; supplied engines retain their owner.
export class SyncServer<
  S extends Schema,
  P extends Principal,
> implements AsyncDisposable {
  private closing?: Promise<void>
  private readonly attachments = new Set<Attachment>()

  constructor(
    readonly application: Application<S, P>,
    readonly options: ServerOptions<S, P>,
    private readonly owner?: OwnedEngine,
  ) {}

  as(principal: P): DatabaseAccess<S> & AppSource<S> {
    return this.application.access(principal)
  }

  admin(scope: string): DatabaseAccess<S> {
    const principal = { subject: 'admin', scope } as P
    return createAccess((operation, options) =>
      this.application.execute(principal, operation, options, true),
    )
  }

  attach(http: HTTPServer, options?: AttachmentOptions): Attachment {
    if (this.closing) throw new SyncError('CLOSED', 'Server is closed')
    const attachment = attachListener(
      http,
      this.application,
      this.options.authenticate,
      options,
    )
    this.attachments.add(attachment)
    return this.track(attachment)
  }

  async listen(options?: ListenerOptions): Promise<Listener> {
    if (this.closing) throw new SyncError('CLOSED', 'Server is closed')
    const listener = await listen(
      this.application,
      this.options.authenticate,
      options,
    )
    if (this.closing) {
      await listener.close()
      throw new SyncError('CLOSED', 'Server is closed')
    }
    this.attachments.add(listener)
    return { ...this.track(listener), url: listener.url }
  }

  close(): Promise<void> {
    this.closing ??= (async () => {
      await Promise.all(
        Array.from(this.attachments, (attachment) => attachment.close()),
      )
      await this.application.close()
      await this.owner?.close()
    })()
    return this.closing
  }

  [Symbol.asyncDispose](): Promise<void> {
    return this.close()
  }

  private track(attachment: Attachment): Attachment {
    const close = async () => {
      try {
        await attachment.close()
      } finally {
        this.attachments.delete(attachment)
      }
    }
    return { close, [Symbol.asyncDispose]: close }
  }
}

// createServer opens and checks storage before returning an admission capability.
export async function createServer<S extends Schema, P extends Principal>(
  options: ServerOptions<S, P>,
): Promise<SyncServer<S, P>> {
  let owner: OwnedEngine | undefined
  let application: Application<S, P> | undefined
  try {
    if (options.directory !== undefined)
      owner = await openNodeEngine(options.directory)
    const engine = options.engine ?? owner?.engine
    if (!engine)
      throw new SyncError(
        'VALIDATION',
        'A directory or supplied engine is required',
      )
    application = new Application({ ...options, engine })
    await application.initialize(options.migration)
    return new SyncServer(application, options, owner)
  } catch (error) {
    await application?.close()
    await owner?.close().catch(() => {})
    throw publicError(error)
  }
}
