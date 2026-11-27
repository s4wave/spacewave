import { createServer as createServerImpl } from '../../core/sync/server.js'
import type { Principal, Schema } from '../../sdk/sync/schema.js'
import type { ServerOptions, SyncServer } from '../../core/sync/config.js'

// The package exposes the hosting contract. Supplied Engine composition stays
// in core.
export const createServer: <S extends Schema, P extends Principal>(
  options: ServerOptions<S, P>,
) => Promise<SyncServer<S, P>> = createServerImpl

export type {
  Access,
  Attachment,
  AttachmentOptions,
  Authenticate,
  Listener,
  ListenerOptions,
  Migration,
  ServerOptions,
  SyncServer,
} from '../../core/sync/config.js'
