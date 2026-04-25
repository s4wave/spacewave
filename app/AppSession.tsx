import { useCallback, useMemo } from 'react'

import {
  resolvePath,
  useNavigate,
  useParams,
  type To,
} from '@s4wave/web/router/router.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { markInteracted } from '@s4wave/web/state/interaction.js'
import {
  SessionIndexContext,
  SessionRouteContext,
} from '@s4wave/web/contexts/contexts.js'
import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import { SessionContainer } from './session/SessionContainer.js'
import { PinUnlockOverlay } from './session/PinUnlockOverlay.js'
import {
  SessionLockMode,
  type EntityCredential,
} from '@s4wave/core/session/session.pb.js'
import type { Root } from '@s4wave/sdk/root/root.js'

// AppSession handles the /u/{session-idx}/* path.
export function AppSession() {
  const navigate = useNavigate()
  const { sessionIndex: sessionIndexParam } = useParams()

  const sessionIdx = parseInt(sessionIndexParam ?? '') || null
  const sessionBasePath = sessionIdx ? `/u/${sessionIdx}` : '/u'

  const rootResource = useRootResource()
  const metadata = useSessionMetadata(sessionIdx)

  const isPinLocked = metadata?.lockMode === SessionLockMode.PIN_ENCRYPTED

  const handleUnlock = useCallback(
    async (pin: Uint8Array) => {
      const root = rootResource.value
      if (!root || !sessionIdx) return
      await root.unlockSession(sessionIdx, pin)
    },
    [rootResource.value, sessionIdx],
  )

  // Always try to mount. If the session is already unlocked (or auto-unlock),
  // mount succeeds immediately. If PIN-locked, mount blocks until unlock.
  const sessionResource = useResource(
    rootResource,
    async (root: Root, signal, cleanup) => {
      if (!sessionIdx) return null
      const result = await root.mountSessionByIdx({ sessionIdx }, signal)
      if (result === null) {
        console.warn(
          'session idx returned not found, redirecting to /',
          sessionIdx,
        )
        queueMicrotask(() => navigate({ path: '/', replace: true }))
        return null
      }
      return cleanup(result.session)
    },
    [sessionIdx],
    {
      onSuccess: () => {
        markInteracted()
      },
    },
  )

  const handleReset = useCallback(
    async (idx: number, credential: EntityCredential) => {
      const root = rootResource.value
      if (!root) return
      await root.resetSession(idx, credential)
    },
    [rootResource.value],
  )

  const sessionNavigate = useCallback(
    (to: To) => {
      navigate({
        ...to,
        path: resolvePath(sessionBasePath, to),
      })
    },
    [navigate, sessionBasePath],
  )

  const sessionRoute = useMemo(
    () => ({
      basePath: sessionBasePath,
      navigate: sessionNavigate,
    }),
    [sessionBasePath, sessionNavigate],
  )

  if (!sessionIdx) {
    console.warn('session idx invalid, redirecting to /', sessionIdx)
    queueMicrotask(() => navigate({ path: '/', replace: true }))
    return null
  }

  // Wrap all session-level renders in SessionIndexContext so children
  // can access the session index without parsing the URL.
  let content: React.ReactNode
  if (isPinLocked && !sessionResource.value && metadata) {
    content = (
      <PinUnlockOverlay
        metadata={metadata}
        onUnlock={handleUnlock}
        onReset={handleReset}
      />
    )
  } else if (sessionResource.error) {
    content = (
      <div className="flex min-h-screen w-full items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'error',
              title: 'Failed to load session',
              error: sessionResource.error.message,
              onRetry: sessionResource.retry,
            }}
          />
        </div>
      </div>
    )
  } else if (sessionResource.loading || !sessionResource.value) {
    content = (
      <div className="flex min-h-screen w-full items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'loading',
              title: 'Loading session',
              detail: 'Mounting the session and its resources.',
            }}
          />
        </div>
      </div>
    )
  } else {
    content = (
      <SessionContainer
        sessionResource={sessionResource}
        metadata={metadata ?? undefined}
      />
    )
  }

  return (
    <SessionIndexContext.Provider value={sessionIdx}>
      <SessionRouteContext.Provider value={sessionRoute}>
        {content}
      </SessionRouteContext.Provider>
    </SessionIndexContext.Provider>
  )
}
