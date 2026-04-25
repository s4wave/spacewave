import { useCallback, useMemo } from 'react'
import { useWatchStateRpc } from '@aptre/bldr-react'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

import { RootContext, SessionContext } from '@s4wave/web/contexts/contexts.js'
import {
  useBackendStateAtomValue,
  type StateAtomAccessor,
} from '@s4wave/web/state/useBackendStateAtom.js'
import {
  WatchStateAtomsRequest,
  WatchStateAtomsResponse,
} from '@s4wave/sdk/root/root.pb.js'
import {
  WatchSessionStateAtomsRequest,
  WatchSessionStateAtomsResponse,
} from '@s4wave/sdk/session/session.pb.js'

import {
  useAtomValue,
  useStateAtoms,
  type StateAtomEntry,
} from './StateDevToolsContext.js'

export type StateInspectorScope = 'local' | 'persistent' | 'root' | 'session'

export type StateInspectorEntry =
  | {
      kind: 'legacy'
      id: string
      label: string
      scope: 'local' | 'persistent'
      atom: StateAtomEntry['atom']
    }
  | {
      kind: 'resource'
      id: string
      label: string
      scope: 'root' | 'session'
      storeId: string
    }

const NULL_ACCESSOR: StateAtomAccessor = {
  value: null,
  loading: false,
  error: null,
  retry: () => {},
}

export function useStateInspectorEntries(): StateInspectorEntry[] {
  const legacyAtoms = useStateAtoms()
  const rootStoreIds = useWatchRootStateAtomStoreIds()
  const sessionStoreIds = useWatchSessionStateAtomStoreIds()

  return useMemo(() => {
    const entries: StateInspectorEntry[] = []
    for (const entry of legacyAtoms.values()) {
      entries.push({
        kind: 'legacy',
        id: `${entry.scope}:${entry.id}`,
        label: entry.name,
        scope: entry.scope,
        atom: entry.atom,
      })
    }
    for (const storeId of rootStoreIds) {
      entries.push({
        kind: 'resource',
        id: `root:${storeId}`,
        label: storeId,
        scope: 'root',
        storeId,
      })
    }
    for (const storeId of sessionStoreIds) {
      entries.push({
        kind: 'resource',
        id: `session:${storeId}`,
        label: storeId,
        scope: 'session',
        storeId,
      })
    }
    return entries
  }, [legacyAtoms, rootStoreIds, sessionStoreIds])
}

export function useStateInspectorEntryMap(): Map<string, StateInspectorEntry> {
  const entries = useStateInspectorEntries()
  return useMemo(
    () => new Map(entries.map((entry) => [entry.id, entry])),
    [entries],
  )
}

export function useStateAtomAccessorForScope(
  scope: 'root' | 'session',
): StateAtomAccessor {
  const rootResource = RootContext.useContext()
  const rootValue = useResourceValue(rootResource)
  const sessionResource = SessionContext.useContext()
  const sessionValue = useResourceValue(sessionResource)

  return useMemo(() => {
    if (scope === 'root') {
      if (!rootValue) return NULL_ACCESSOR
      return {
        value: (storeId: string, signal?: AbortSignal) =>
          rootValue.accessStateAtom({ storeId }, signal),
        loading: false,
        error: null,
        retry: () => rootResource.retry(),
      }
    }
    if (!sessionValue) return NULL_ACCESSOR
    return {
      value: (storeId: string, signal?: AbortSignal) =>
        sessionValue.accessStateAtom({ storeId }, signal),
      loading: false,
      error: null,
      retry: () => sessionResource.retry(),
    }
  }, [rootResource, rootValue, scope, sessionResource, sessionValue])
}

export function useStateInspectorValue(entry: StateInspectorEntry): unknown {
  const legacyValue = useAtomValue(entry.kind === 'legacy' ? entry.atom : null)
  const rootAccessor = useStateAtomAccessorForScope('root')
  const sessionAccessor = useStateAtomAccessorForScope('session')
  const resourceAccessor =
    entry.kind === 'resource' ?
      entry.scope === 'root' ?
        rootAccessor
      : sessionAccessor
    : NULL_ACCESSOR
  const resourceState = useBackendStateAtomValue(
    resourceAccessor,
    entry.kind === 'resource' ? entry.storeId : '__devtools-unused__',
    {},
  )
  return entry.kind === 'legacy' ? legacyValue : resourceState.value
}

function useWatchRootStateAtomStoreIds(): string[] {
  const rootResource = RootContext.useContext()
  const rootValue = useResourceValue(rootResource)

  const watchFn = useCallback(
    (_: WatchStateAtomsRequest, signal: AbortSignal) =>
      rootValue?.watchStateAtoms({}, signal) ?? null,
    [rootValue],
  )

  const resp = useWatchStateRpc(
    watchFn,
    {},
    WatchStateAtomsRequest.equals,
    WatchStateAtomsResponse.equals,
  )
  return resp?.storeIds ?? []
}

function useWatchSessionStateAtomStoreIds(): string[] {
  const sessionResource = SessionContext.useContext()
  const sessionValue = useResourceValue(sessionResource)

  const watchFn = useCallback(
    (_: WatchSessionStateAtomsRequest, signal: AbortSignal) =>
      sessionValue?.watchStateAtoms({}, signal) ?? null,
    [sessionValue],
  )

  const resp = useWatchStateRpc(
    watchFn,
    {},
    WatchSessionStateAtomsRequest.equals,
    WatchSessionStateAtomsResponse.equals,
  )
  return resp?.storeIds ?? []
}
