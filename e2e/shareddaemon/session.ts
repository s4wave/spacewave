import type { Client as RpcClient } from 'starpc'

import { Client as ResourceClient } from '../../bldr/sdk/resource/client.js'
import { ResourceServiceClient } from '../../bldr/resource/resource_srpc.pb.js'
import { RootResourceServiceClient } from '../../sdk/root/root_srpc.pb.js'
import { SessionResourceServiceClient } from '../../sdk/session/session_srpc.pb.js'

/** openSession mounts the sole fixture Session for the caller's window or worker lifetime. */
export async function openSession(rpc: RpcClient) {
  // Connect the Resource client through this window or worker route.
  const abort = new AbortController()
  const resources = new ResourceClient(
    new ResourceServiceClient(rpc),
    abort.signal,
  )
  const rootRef = await resources.accessRootResource()
  const root = new RootResourceServiceClient(rootRef.client)

  // Require the sole CLI-created Session before mounting it.
  const list = await root.ListSessions({})
  const sessions = list.sessions ?? []
  if (sessions.length !== 1) {
    throw new Error(`Session count = ${sessions.length}, want 1`)
  }
  const entry = sessions[0]!

  // Mount the Session and retain its Resource reference.
  const mounted = await root.MountSessionByIdx({
    sessionIdx: entry.sessionIndex,
  })
  if (!mounted.resourceId || mounted.notFound) {
    throw new Error('fixture Session was not mounted')
  }
  const sessionRef = resources.createResourceReference(mounted.resourceId)
  rootRef.release()
  const session = new SessionResourceServiceClient(sessionRef.client)

  // Read one Space snapshot and close the frontend watch.
  const snapshotAbort = new AbortController()
  const watch = session
    .WatchResourcesList({}, snapshotAbort.signal)
    [Symbol.asyncIterator]()
  const initial = await watch.next()
  snapshotAbort.abort()
  const spaces = initial.done ? [] : (initial.value.spacesList ?? [])
  if (spaces.length !== 1) {
    throw new Error('fixture Space was not found')
  }

  // Return the Session service with the same stored identities as the CLI.
  const space = spaces[0]!
  const sessionId = entry.sessionRef?.providerResourceRef?.id
  const spaceId = space.entry?.ref?.providerResourceRef?.id
  if (!sessionId || !spaceId) {
    throw new Error('fixture identity is incomplete')
  }
  return {
    session,
    sessionIndex: entry.sessionIndex,
    sessionId,
    spaceId,
    spaceName: space.spaceMeta?.name || '',
  }
}
