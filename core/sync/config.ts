import type { Server as HTTPServer } from 'node:http'
import type { DatabaseAccess } from '../../sdk/sync/access.js'
import type { AppSource } from '../../sdk/sync/app.js'
import type {
  MutationHandlers,
  Principal,
  Schema,
  Transaction,
} from '../../sdk/sync/schema.js'

export interface Access<P extends Principal> {
  readonly principal: P
  readonly scope: string
  readonly collection: string
  readonly action: 'read' | 'write'
}

export interface ApplicationConfig<S extends Schema, P extends Principal> {
  schema: S
  /** instance selects one application dataset in the supplied World; defaults to schema.id. */
  instance?: string
  authorize(access: Access<P>): boolean | Promise<boolean>
  mutations: MutationHandlers<S, P>
  limits?: {
    maxRecords?: number
    maxSnapshotBytes?: number
    maxRecordBytes?: number
  }
}

export interface Migration<S extends Schema> {
  from: number
  run(context: {
    scopes(): Promise<readonly string[]>
    scope(scope: string): Transaction<S>
    signal: AbortSignal
  }): Promise<void>
}

export type Authenticate<P extends Principal> = (
  token: string,
  signal: AbortSignal,
) => Promise<P>

export interface ServerOptions<
  S extends Schema,
  P extends Principal,
> extends ApplicationConfig<S, P> {
  directory: string
  authenticate: Authenticate<P>
  migration?: Migration<S>
}

export interface AttachmentOptions {
  path?: string
  allowedOrigins?: readonly string[]
  maxRequestBytes?: number
}

export interface ListenerOptions extends AttachmentOptions {
  port?: number
  host?: string
}

export interface Attachment extends AsyncDisposable {
  close(): Promise<void>
}

export interface Listener extends Attachment {
  readonly url: string
}

// SyncServer grants scoped access and owns only the listeners and engine it opened.
export interface SyncServer<
  S extends Schema,
  P extends Principal,
> extends AsyncDisposable {
  as(principal: P): DatabaseAccess<S> & AppSource<S>
  admin(scope: string): DatabaseAccess<S>
  attach(http: HTTPServer, options?: AttachmentOptions): Attachment
  listen(options?: ListenerOptions): Promise<Listener>
  close(): Promise<void>
}
