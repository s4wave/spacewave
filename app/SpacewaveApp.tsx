import { useCallback, useEffect, useRef, useState } from 'react'
import type { Client as ResourceClient } from '@aptre/bldr-sdk/resource/client.js'
import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'
import { Root } from '@s4wave/sdk/root'
import type { AppRuntime, AppStorage } from '@s4wave/sdk/root/app.js'
import {
  clearDebugContext,
  setDebugContext,
} from '@s4wave/sdk/debug/context.js'
import {
  useRootResource,
  useRootResourceClient,
} from '@s4wave/web/hooks/useRootResource.js'
import {
  AppEnvironmentContext,
  bindRootEnvironment,
  createAppEnvironment,
  type AppEnvironment,
} from '@s4wave/web/sdk/app/environment.js'
import { persistAppEnvironment } from '@s4wave/web/sdk/app/persistence.js'
import { TooltipProvider } from '@s4wave/web/ui/tooltip.js'
import { LoadingScreen } from '@s4wave/web/ui/loading/LoadingScreen.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { AppAPI } from './AppAPI.js'
import { releaseQuickstartAppHandoffs } from './quickstart/session-handoff.js'
import { EditorShell } from './EditorShell.js'

export interface AppSetupContext {
  root: Root
  client: ResourceClient
  environment: AppEnvironment
  signal: AbortSignal
  cleanup: RegisterCleanup
  reportProgress: (detail: string) => void
}

export interface AppSetupResult {
  path: string
  debug?: Record<string, unknown>
}

export interface SpacewaveAppProps {
  appId: string
  suppliedStorage: AppStorage
  setup?: (context: AppSetupContext) => Promise<AppSetupResult>
}

interface RunningApp {
  runtime: AppRuntime
  environment: AppEnvironment
  debug: Record<string, unknown>
  setup: SpacewaveAppProps['setup']
  storageId: string | undefined
  world: Extract<AppStorage, { world: unknown }>['world'] | undefined
  objectPrefix: string | undefined
}

// SpacewaveApp mounts the ordinary app on supplied storage. Its attachment
// owns startup, seeded resources, and teardown; the caller owns persistent data.
export function SpacewaveApp({
  appId,
  suppliedStorage,
  setup,
}: SpacewaveAppProps) {
  const parentRoot = useRootResource().value
  const parentClient = useRootResourceClient()
  const [generation, setGeneration] = useState(0)
  const [running, setRunning] = useState<RunningApp | null>(null)
  const [error, setError] = useState<Error | null>(null)
  const [progress, setProgress] = useState('Opening app storage')
  const teardown = useRef(Promise.resolve())
  const resetWaiters = useRef<
    Array<{ resolve(): void; reject(error: unknown): void }>
  >([])
  const reset = useCallback(() => {
    setRunning(null)
    setError(null)
    setProgress('Opening app storage')
    setGeneration((value) => value + 1)
    return new Promise<void>((resolve, reject) => {
      resetWaiters.current.push({ resolve, reject })
    })
  }, [])
  useEffect(
    () => () => {
      for (const waiter of resetWaiters.current.splice(0))
        waiter.reject(new Error('App unmounted during reset'))
    },
    [],
  )
  const storageId =
    'storageId' in suppliedStorage ? suppliedStorage.storageId : undefined
  const world = 'world' in suppliedStorage ? suppliedStorage.world : undefined
  const objectPrefix =
    'objectPrefix' in suppliedStorage ? suppliedStorage.objectPrefix : undefined

  useEffect(() => {
    if (!parentRoot || !parentClient) return
    setRunning(null)
    setError(null)
    const controller = new AbortController()
    const environment = createAppEnvironment(appId)
    const resources: Array<{ [Symbol.dispose](): void }> = []
    let closed = false
    let runtime: AppRuntime | undefined
    let unbind: (() => void) | undefined
    let flushEnvironment: (() => Promise<void>) | undefined
    let debug: Record<string, unknown> = {
      status: 'loading',
      reset,
      generation,
      environment,
    }
    const cleanup: RegisterCleanup = (resource) => {
      if (resource) {
        if (closed) resource[Symbol.dispose]()
        else resources.push(resource)
      }
      return resource
    }
    const disposeResources = () => {
      closed = true
      for (const resource of resources.reverse()) resource[Symbol.dispose]()
      resources.length = 0
      unbind?.()
      releaseQuickstartAppHandoffs(environment.instanceKey)
    }
    const previousTeardown = teardown.current
    const start = async () => {
      await previousTeardown
      controller.signal.throwIfAborted()
      setProgress('Opening app storage')
      setDebugContext(debug, appId)
      runtime = await parentRoot.mountApp(
        world
          ? { world, objectPrefix: objectPrefix ?? '' }
          : storageId !== undefined
            ? { storageId }
            : { ephemeral: true },
        controller.signal,
      )
      environment.httpPathPrefix = runtime.httpPathPrefix
      const root = cleanup(
        new Root(await runtime.resourceClient.accessRootResource()),
      )
      controller.signal.throwIfAborted()
      const releaseListener = runtime.resourceClient.onResourceReleased(
        (event) => {
          if (controller.signal.aborted || event.resourceId !== root.id) return
          const failure = new Error('App connection closed')
          controller.abort()
          debug = { ...debug, status: 'error', error: failure.message }
          setDebugContext(debug, appId)
          setRunning(null)
          setError(failure)
          for (const waiter of resetWaiters.current.splice(0))
            waiter.reject(failure)
        },
      )
      cleanup({ [Symbol.dispose]: releaseListener })
      if (world || storageId !== undefined) {
        flushEnvironment = await persistAppEnvironment(
          root,
          environment,
          controller.signal,
          (cause) =>
            setError(cause instanceof Error ? cause : new Error(String(cause))),
        )
      }
      unbind = bindRootEnvironment(root, environment)
      debug = { ...debug, root, client: runtime.resourceClient }
      setDebugContext(debug, appId)
      const result = await setup?.({
        root,
        client: runtime.resourceClient,
        environment,
        signal: controller.signal,
        cleanup,
        reportProgress: (detail) => {
          if (!controller.signal.aborted) setProgress(detail)
        },
      })
      controller.signal.throwIfAborted()
      await runtime.resourceClient.waitForControls(controller.signal)
      if (result) environment.navigation.setAppPath(result.path)
      debug = { ...debug, ...result?.debug, status: 'ready' }
      setDebugContext(debug, appId)
      setRunning({
        runtime,
        environment,
        debug,
        setup,
        storageId,
        world,
        objectPrefix,
      })
      setError(null)
      for (const waiter of resetWaiters.current.splice(0)) waiter.resolve()
    }
    const started = start().catch((cause: unknown) => {
      if (controller.signal.aborted) return
      const failure = cause instanceof Error ? cause : new Error(String(cause))
      debug = { ...debug, status: 'error', error: failure.message }
      setDebugContext(debug, appId)
      setError(failure)
      for (const waiter of resetWaiters.current.splice(0))
        waiter.reject(failure)
    })
    return () => {
      controller.abort()
      disposeResources()
      clearDebugContext(debug, appId)
      teardown.current = started.then(async () => {
        disposeResources()
        if (!runtime) return
        await flushEnvironment?.().catch((cause: unknown) => {
          console.error('Failed to save app state', cause)
        })
        // The existing Resource control ACK waits for server release callbacks.
        await runtime.resourceClient.waitForControls().catch(() => {})
        runtime.release()
        await parentClient.waitForControls().catch(() => {})
      })
    }
  }, [
    appId,
    generation,
    parentRoot,
    parentClient,
    reset,
    setup,
    storageId,
    world,
    objectPrefix,
  ])

  const current =
    running?.environment.id === appId &&
    running.setup === setup &&
    running.storageId === storageId &&
    running.world === world &&
    running.objectPrefix === objectPrefix
      ? running
      : null

  return (
    <div
      data-spacewave-app={appId}
      data-app-status={error ? 'error' : current ? 'ready' : 'loading'}
      className="relative flex h-full min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
      tabIndex={-1}
    >
      {error ? (
        <ErrorState
          title="App setup failed"
          message={error.message}
          onRetry={() => {
            void reset().catch(() => {})
          }}
        />
      ) : current ? (
        <AppEnvironmentContext.Provider value={current.environment}>
          <TooltipProvider>
            <AppAPI
              key={generation}
              resourceClient={current.runtime.resourceClient}
              rootAtom={current.environment.rootAtom}
              debugContext={current.debug}
            >
              <EditorShell />
            </AppAPI>
          </TooltipProvider>
        </AppEnvironmentContext.Provider>
      ) : (
        <LoadingScreen
          view={{
            state: 'loading',
            title: 'Preparing your workspace',
            detail: progress,
          }}
        />
      )}
    </div>
  )
}
