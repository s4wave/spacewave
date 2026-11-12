// Import types generated from protobuf definitions.
import { Client, isAbortError } from 'starpc'
import type { JsonValue } from '@aptre/protobuf-es-lite'
import {
  isBackendEntrypointFunc,
  isBackendEntrypointLifecycle,
  type BackendAPI,
  type BackendEntrypointLifecycle,
} from '@aptre/bldr-sdk'
import { BackendEntrypoint, Config, FrontendEntrypoint } from './compiler.pb.js'
import { ConfigSet } from '@go/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.js'
import {
  SetHtmlLinksRequest,
  SetRenderModeRequest,
} from '../../../web/view/view.pb.js'
import { retryWithAbort } from '../../../web/bldr/retry.js'
import { PluginHostResourceServiceClient } from '../../../sdk/plugin/host/host_srpc.pb.js'
import { createAbortController } from '../../../web/bldr/abort.js'
import {
  WebPlugin,
  WebPluginClient,
} from '../../../web/plugin/plugin_srpc.pb.js'
import { WebViewHandlerConfig } from '../../../web/view/handler/handler.pb.js'
import {
  HandleWebPkgsViaPluginAssetsRequest,
  HandleWebViewViaHandlersRequest,
} from '../../../web/plugin/plugin.pb.js'

// Defines the list of backend entrypoints to load.
declare const __BLDR_BACKEND_ENTRYPOINTS__: BackendEntrypoint[] | undefined

// Defines the list of frontend entrypoints.
declare const __BLDR_FRONTEND_ENTRYPOINTS__: FrontendEntrypoint[] | undefined

// Defines the set of config set to apply to the plugin host.
declare const __BLDR_HOST_CONFIG_SET__: ConfigSet['configs'] | undefined

// Defines the ID of the plugin serving the WebRuntime APIs.
declare const __BLDR_WEB_PLUGIN_ID__: string | undefined

// Defines the request to send to the web plugin to serve web pkgs.
//
// handle_plugin_id is overridden at runtime.
declare const __BLDR_HANDLE_WEB_PKGS__:
  | HandleWebPkgsViaPluginAssetsRequest
  | undefined

const quickJSPluginFrontendReadyMarker =
  '__BLDR_QUICKJS_PLUGIN_FRONTEND_READY__'
const quickJSPluginCapabilityReadyMarker =
  '__BLDR_QUICKJS_PLUGIN_CAPABILITY_READY__'
const quickJSPluginReadyMarker = '__BLDR_QUICKJS_PLUGIN_READY__'

/**
 * Logs an error message and the full error object consistently.
 * @param message - The base error message.
 * @param error - The error object to log.
 */
function logError(message: string, error: unknown): void {
  let errMsg = message
  if (error instanceof Error) {
    errMsg += ': ' + error.message
  }
  console.error(errMsg)
  console.error(error)
}

export function isEntrypointLifecycleRetry(error: unknown): boolean {
  if (error !== null && isAbortError(error)) return true
  if (!(error instanceof Error)) return false
  return error.name === 'StreamResetError' || error.message === 'stream reset'
}

export function entrypointRetryOpts(message: string) {
  return {
    errorCb(error: unknown): void {
      if (isEntrypointLifecycleRetry(error)) return
      logError(message, error)
    },
  }
}

function isQuickJSRuntime(): boolean {
  return 'std' in globalThis
}

function resolveBackendEntrypointImportPath(
  importPath: string,
  backendAPI: BackendAPI,
): string {
  if (isQuickJSRuntime() || !importPath.startsWith('/assets/')) {
    return importPath
  }

  const pluginId = backendAPI.startInfo.pluginId
  if (!pluginId) {
    return importPath
  }
  return backendAPI.utils.pluginAssetHttpPath(
    pinnedPluginArtifactID(backendAPI, pluginId),
    importPath.slice('/assets/'.length),
  )
}

// pinnedPluginArtifactID binds asset URLs to the worker's immutable manifest,
// so backend imports, including replay, and the documents it renders keep
// loading the release they started with. Frontend file names carry no content
// hash, so the manifest root is what tells releases apart.
function pinnedPluginArtifactID(
  backendAPI: BackendAPI,
  pluginId: string,
): string {
  const root = backendAPI.startInfo.manifestRoot
  if (!root) throw new Error('Backend worker has no immutable manifest binding')
  return `${pluginId}/manifest/${root}`
}

type BackendEntrypointModule = Record<string, unknown>
export type BackendEntrypointModuleLoader = (
  importPath: string,
) => Promise<BackendEntrypointModule>

function importBackendEntrypointModule(
  importPath: string,
): Promise<BackendEntrypointModule> {
  // The import path is relative to the assets FS root (e.g., /p/{plugin-id}/a/).
  // Example: vite/backend/index.js or esb/backend/index.js
  // The host environment must resolve these paths relative to the assets base URL.
  return import(/* @vite-ignore */ importPath)
}

// BackendEntrypointFailureFunc receives a started entrypoint's failure. It is
// not called when the entrypoint ends because the plugin was aborted.
export type BackendEntrypointFailureFunc = (error: unknown) => void

function observeBackendEntrypointCompletion(
  entrypointId: string,
  entrypointPromise: Promise<void> | void,
  abortSignal: AbortSignal,
  onFailure: BackendEntrypointFailureFunc,
): void {
  Promise.resolve(entrypointPromise)
    .then(() => {
      console.debug(`Backend entrypoint finished: ${entrypointId}`)
    })
    .catch((error: unknown) => {
      // A swap or shutdown aborts every entrypoint; that is not a failure.
      if (abortSignal.aborted || isAbortError(error)) {
        console.debug(`Backend entrypoint aborted: ${entrypointId}`)
        return
      }
      logError(`Backend entrypoint failed after startup ${entrypointId}`, error)
      onFailure(error)
    })
}

/**
 * Loads and executes a single backend entrypoint module.
 * @param entrypoint - The backend entrypoint configuration.
 * @param backendAPI - The backend API object to pass to the entrypoint function.
 * @param abortSignal - The abort signal to pass to the entrypoint function.
 * @param loadModule - Imports the entrypoint's module.
 * @param onFailure - Receives a failure of the entrypoint after startup.
 * @returns A promise that resolves when the entrypoint function starts, or rejects on startup error.
 */
export async function startBackendEntrypoint(
  entrypoint: BackendEntrypoint,
  backendAPI: BackendAPI,
  abortSignal: AbortSignal,
  loadModule: BackendEntrypointModuleLoader = importBackendEntrypointModule,
  onFailure: BackendEntrypointFailureFunc = () => {},
): Promise<void> {
  // Require a module path before resolving the worker's immutable asset URL.
  if (!entrypoint?.importPath) {
    throw new Error(
      `Invalid backend entrypoint object: ${JSON.stringify(entrypoint)}`,
    )
  }

  // Resolve the module and export against the same pinned worker manifest.
  const importPath = resolveBackendEntrypointImportPath(
    entrypoint.importPath,
    backendAPI,
  )
  const importName = entrypoint.importName || 'default'
  const entrypointId = `${importPath}#${importName}`

  // Start the imported module and wait for its declared startup contract.
  console.debug(`Importing backend module: ${entrypointId}`)
  try {
    const mod = await loadModule(importPath)
    const modFunc = mod[importName]

    if (!isBackendEntrypointFunc(modFunc)) {
      // Backend readiness must fail closed: a configured entrypoint owns the
      // capability startup marker, so a missing export cannot be treated as
      // successful startup.
      throw new Error(
        `Backend entrypoint function '${importName}' not found or not a function in module: ${importPath}`,
      )
    }

    console.debug(`Executing backend entrypoint: ${entrypointId}`)
    const entrypointResult = modFunc(backendAPI, abortSignal)
    if (isBackendEntrypointLifecycle(entrypointResult)) {
      await entrypointResult.startup
      observeBackendEntrypointCompletion(
        entrypointId,
        entrypointResult.done,
        abortSignal,
        onFailure,
      )
      return
    }
    observeBackendEntrypointCompletion(
      entrypointId,
      entrypointResult,
      abortSignal,
      onFailure,
    )
  } catch (error) {
    logError(
      `Failed to load or start backend entrypoint ${entrypointId}`,
      error,
    )
    return Promise.reject(error)
  }
}

/**
 * loadBackendEntrypoints starts each configured backend module in order.
 * It resolves after startup, while module lifetimes continue until abort;
 * onFailure receives the first module that fails after startup.
 */
export async function loadBackendEntrypoints(
  backendAPI: BackendAPI,
  abortSignal: AbortSignal,
  backendEntrypoints: BackendEntrypoint[] = [],
  loadModule: BackendEntrypointModuleLoader = importBackendEntrypointModule,
  onFailure: BackendEntrypointFailureFunc = () => {},
): Promise<void> {
  // Empty module sets make backend readiness immediate.
  if (backendEntrypoints.length === 0) {
    console.debug('No backend entrypoints configured.')
    return
  }

  // Wait for every backend entrypoint to import and start. The entrypoint
  // lifecycle promises may intentionally stay pending until plugin shutdown.
  console.debug(
    `Waiting for ${backendEntrypoints.length} backend entrypoints to start...`,
  )
  for (const entrypoint of backendEntrypoints) {
    // Startup failure means this plugin's backend capabilities are not ready.
    // Let the caller suppress ready markers instead of reporting a partial boot.
    await startBackendEntrypoint(
      entrypoint,
      backendAPI,
      abortSignal,
      loadModule,
      onFailure,
    )
  }
  console.debug('All backend entrypoints started successfully.')
}

/**
 * loadWebPkgs registers asset packages until abort and resolves on readiness.
 */
export async function loadWebPkgs(
  ourPluginID: string,
  webPlugin: WebPlugin,
  abortSignal: AbortSignal,
  onReady?: () => void,
  handleWebPkgs: HandleWebPkgsViaPluginAssetsRequest | undefined = undefined,
): Promise<void> {
  // Empty package sets need no registration before frontend startup.
  const webPkgsIDs = handleWebPkgs?.webPkgIdList
  if (!webPkgsIDs?.length) {
    console.debug('No web pkgs configured.')
    onReady?.()
    return
  }

  // Bind the package handler to this plugin's immutable asset identity.
  const request = HandleWebPkgsViaPluginAssetsRequest.clone(handleWebPkgs)!
  request.handlePluginId = ourPluginID

  // Retain the first readiness result while the handler stream stays live.
  const { promise, resolve, reject } = Promise.withResolvers<void>()
  let ready = false
  const cleanup = () => {
    abortSignal.removeEventListener('abort', abortReady)
  }
  const resolveReady = () => {
    // Complete startup once and detach the startup-only abort listener.
    if (ready) return
    ready = true
    cleanup()
    onReady?.()
    resolve()
  }
  const rejectReady = (error: unknown) => {
    if (ready) {
      if (!abortSignal.aborted) {
        logError('Web package handler failed after startup', error)
      }
      return
    }
    cleanup()
    reject(error)
  }
  const abortReady = () => {
    rejectReady(new Error('web package setup aborted'))
  }

  // Keep the package stream registered for the plugin's lifetime.
  abortSignal.addEventListener('abort', abortReady, { once: true })
  void retryWithAbort(
    abortSignal,
    async (signal) => {
      const response = webPlugin.HandleWebPkgsViaPluginAssets(request, signal)
      for await (const result of response) {
        if (result.body?.case !== 'ready') continue
        const isReady = result.body.value || false
        if (isReady) {
          console.debug(
            `Configured ${webPkgsIDs.length} web pkgs via web plugin.`,
          )
          resolveReady()
          continue
        }
        console.debug('Web plugin is not ready yet.')
      }
    },
    entrypointRetryOpts('error configuring web packages'),
  )
    .then(() => {
      if (!ready) {
        rejectReady(new Error('web package stream closed before readiness'))
      }
    })
    .catch(rejectReady)
  return promise
}

/**
 * loadFrontendEntrypoints registers the plugin's render and stylesheet handlers.
 */
async function loadFrontendEntrypoints(
  backendAPI: BackendAPI,
  ourPluginID: string,
  webPlugin: WebPlugin,
  abortSignal: AbortSignal,
  config: Config,
  handleWebPkgs: HandleWebPkgsViaPluginAssetsRequest | undefined,
  onReady?: () => void,
): Promise<void> {
  // Start only the frontend views in this plugin's compiled configuration.
  const frontendEntrypoints = config.frontendEntrypoints ?? []
  if (frontendEntrypoints.length === 0) {
    console.debug('No frontend entrypoints configured.')
    onReady?.()
    return
  }

  // Build view handlers against the plugin's immutable asset paths.
  const handlers: WebViewHandlerConfig[] = []
  for (const entrypoint of frontendEntrypoints) {
    if (!entrypoint) continue

    // Add to the list of handlers.
    const pushHandler = (handler: WebViewHandlerConfig['handler']) =>
      handlers.push({
        handler,
        webViewId: entrypoint.webViewId,
        webViewParentId: entrypoint.webViewParentId,
      })

    // Check if empty and clone by serializing to json
    const setRenderModeRequestBin = entrypoint.setRenderMode
      ? SetRenderModeRequest.toBinary(entrypoint.setRenderMode)
      : null
    if (setRenderModeRequestBin?.length) {
      // Clone the message via fromBinary
      const setRenderModeRequest = SetRenderModeRequest.fromBinary(
        setRenderModeRequestBin,
      )

      // Bind this plugin's packages to the same manifest as its renderer.
      // The web plugin resolves packages supplied by other plugins.
      for (const id of handleWebPkgs?.webPkgIdList ?? []) {
        if (id in (setRenderModeRequest.webPkgPaths ?? {})) {
          setRenderModeRequest.webPkgPaths![id] =
            backendAPI.utils.pluginAssetHttpPath(
              pinnedPluginArtifactID(backendAPI, ourPluginID),
              `${handleWebPkgs!.webPkgsPath}/${id}/`,
            )
        }
      }

      // Override the script path to be /b/pa/{plugin-id}/manifest/{root}/...
      if (
        setRenderModeRequest.scriptPath &&
        !setRenderModeRequest.frontendBinding
      ) {
        setRenderModeRequest.scriptPath = backendAPI.utils.pluginAssetHttpPath(
          pinnedPluginArtifactID(backendAPI, ourPluginID),
          setRenderModeRequest.scriptPath,
        )
      }

      // Set the handler
      pushHandler({ case: 'setRenderMode', value: setRenderModeRequest })
    }

    // Check if empty and clone by serializing to json
    const setHtmlLinksRequestBin = entrypoint.setHtmlLinks
      ? SetHtmlLinksRequest.toBinary(entrypoint.setHtmlLinks)
      : null
    if (setHtmlLinksRequestBin?.length) {
      const setHtmlLinksRequest = SetHtmlLinksRequest.fromBinary(
        setHtmlLinksRequestBin,
      )

      // Override the href paths to be /b/pa/{plugin-id}/manifest/{root}/...
      if (setHtmlLinksRequest.setLinks) {
        for (const link of Object.values(setHtmlLinksRequest.setLinks)) {
          if (link?.href) {
            link.href = backendAPI.utils.pluginAssetHttpPath(
              pinnedPluginArtifactID(backendAPI, ourPluginID),
              link.href,
            )
          }
        }
      }

      pushHandler({ case: 'setHtmlLinks', value: setHtmlLinksRequest })
    }
  }

  // Resolve empty handler sets without opening a stream to the web plugin.
  if (!handlers.length) {
    console.debug(`No web view handlers were configured.`)
    onReady?.()
    return
  }

  // Send the render and stylesheet handlers as one configuration.
  const handlersRequest: HandleWebViewViaHandlersRequest = {
    config: { handlers },
  }
  console.debug(
    `Configuring ${handlers.length} web view handlers: ${HandleWebViewViaHandlersRequest.toJsonString(handlersRequest)}`,
  )

  // Keep view handlers registered until abort and report their readiness.
  await retryWithAbort(
    abortSignal,
    async (signal) => {
      const response = webPlugin.HandleWebViewViaHandlers(
        handlersRequest,
        signal,
      )
      for await (const result of response) {
        if (result.body?.case !== 'ready') continue
        const isReady = result.body.value || false
        if (isReady) {
          console.debug(
            `Configured ${handlers.length} web view handlers via web plugin.`,
          )
          onReady?.()
          continue
        }
        console.debug('Web plugin is not ready yet.')
      }
    },
    entrypointRetryOpts('error configuring web view handlers'),
  )
}

/**
 * Load the web plugin and the frontend entrypoints if any are configured.
 */
function loadWebPlugin(
  backendAPI: BackendAPI,
  ourPluginID: string,
  abortSignal: AbortSignal,
  config: Config,
  handleWebPkgs: HandleWebPkgsViaPluginAssetsRequest | undefined,
): Promise<void> {
  // Resolve the shared web runtime required by this plugin's views.
  const webPluginID = config.webPluginId ?? ''
  if (!webPluginID?.length) {
    console.debug(
      'Skipping frontend entrypoints as no webPluginId was configured.',
    )
    return Promise.resolve()
  }

  // Skip the web runtime when neither views nor asset packages need it.
  if (
    !(config.frontendEntrypoints ?? []).length &&
    !handleWebPkgs?.webPkgIdList?.length
  ) {
    console.debug('No frontend handlers or web pkgs configured.')
    return Promise.resolve()
  }

  // Resolve initial startup only when the view and package handlers are ready.
  console.debug(`Loading web plugin with ID: ${webPluginID}`)
  let pluginAbort: AbortController | undefined = undefined
  return new Promise<void>((resolve, reject) => {
    // Track initial frontend readiness across web plugin restarts.
    const frontend = { ready: false }
    const resolveFrontendReady = () => {
      if (frontend.ready) {
        return
      }
      frontend.ready = true
      abortSignal.removeEventListener('abort', rejectFrontendReady)
      resolve()
    }
    const rejectFrontendReady = () => {
      if (frontend.ready) {
        return
      }
      reject(new Error('frontend setup aborted'))
    }
    abortSignal.addEventListener('abort', rejectFrontendReady, { once: true })

    // Reconfigure handlers each time the shared web plugin starts running.
    function startPluginSetup(signal: AbortSignal) {
      if (pluginAbort) {
        return
      }
      pluginAbort = createAbortController(signal)
      let setupAttempt = 0
      retryWithAbort(
        pluginAbort.signal,
        async (signal) => {
          // Bind this setup attempt and its readiness to the web plugin stream.
          const attempt = ++setupAttempt
          const setupReady = createReadinessBarrier(2, () => {
            if (setupAttempt === attempt) {
              resolveFrontendReady()
            }
          })
          const openStream = backendAPI.buildPluginOpenStream(webPluginID)
          const srpcClient = new Client(openStream)
          const client = new WebPluginClient(srpcClient)
          try {
            // Frontend entrypoint imports can reference /b/pkg assets, so the
            // web plugin must serve this plugin's web packages first.
            await loadWebPkgs(
              pinnedPluginArtifactID(backendAPI, ourPluginID),
              client,
              signal,
              setupReady,
              handleWebPkgs,
            )
            await loadFrontendEntrypoints(
              backendAPI,
              ourPluginID,
              client,
              signal,
              config,
              handleWebPkgs,
              setupReady,
            )
          } catch (err) {
            if (setupAttempt === attempt) {
              setupAttempt++
            }
            throw err
          }
        },
        {
          errorCb: entrypointRetryOpts('error loading frontend entrypoints')
            .errorCb,
        },
      )
    }

    // Follow the web plugin's running state until the parent plugin aborts.
    retryWithAbort(
      abortSignal,
      async (signal) => {
        const respStream = backendAPI.pluginHost.LoadPlugin(
          { pluginId: webPluginID },
          signal,
        )
        for await (const resp of respStream) {
          const currRunning = resp?.pluginStatus?.running || false
          console.debug(`web plugin status running=${currRunning}`)
          if (!currRunning) {
            if (pluginAbort) {
              pluginAbort.abort()
              pluginAbort = undefined
            }
            continue
          }
          startPluginSetup(signal)
        }
      },
      entrypointRetryOpts('error watching web plugin status'),
    )
  })
}

function createReadinessBarrier(
  count: number,
  onReady: () => void,
): () => void {
  const state = { remaining: count }
  return () => {
    if (state.remaining <= 0) {
      return
    }
    state.remaining -= 1
    if (state.remaining === 0) {
      onReady()
    }
  }
}

function reportQuickJSReadiness(marker: string): void {
  if (isQuickJSRuntime()) {
    console.info(marker)
  }
}

async function completeInitialCapabilityRegistration(
  backendAPI: BackendAPI,
  abortSignal: AbortSignal,
): Promise<void> {
  const rootRef = await backendAPI.resourceClient.accessRootResource()
  try {
    const svc = new PluginHostResourceServiceClient(rootRef.client)
    await svc.CompleteInitialCapabilityRegistration({}, abortSignal)
  } finally {
    rootRef.release()
  }
}

/**
 * main starts the backend and frontend modules selected at compile time.
 */
export default function main(
  backendAPI: BackendAPI,
  abortSignal: AbortSignal,
): BackendEntrypointLifecycle {
  return runPluginEntrypoint(
    backendAPI,
    abortSignal,
    {
      backendEntrypoints: __BLDR_BACKEND_ENTRYPOINTS__ ?? [],
      frontendEntrypoints: __BLDR_FRONTEND_ENTRYPOINTS__ ?? [],
      hostConfigSet: __BLDR_HOST_CONFIG_SET__,
      webPluginId: __BLDR_WEB_PLUGIN_ID__,
    },
    __BLDR_HANDLE_WEB_PKGS__,
  )
}

/**
 * createPluginEntrypoint parses a compiled plugin configuration and uses the
 * shared Bldr entrypoint lifecycle for backend and frontend modules.
 */
export function createPluginEntrypoint(configJSON: JsonValue) {
  const config = Config.fromJson(configJSON)
  return (backendAPI: BackendAPI, abortSignal: AbortSignal) =>
    runPluginEntrypoint(backendAPI, abortSignal, config)
}

/**
 * runPluginEntrypoint starts the plugin's configured capabilities and views.
 * Its startup resolves once they are ready; its done rejects with the first
 * failure after startup and otherwise stays pending until the plugin exits,
 * so the host can end the execution as failed.
 */
function runPluginEntrypoint(
  backendAPI: BackendAPI,
  abortSignal: AbortSignal,
  config: Config,
  handleWebPkgs?: HandleWebPkgsViaPluginAssetsRequest,
): BackendEntrypointLifecycle {
  const failed = Promise.withResolvers<void>()
  return {
    startup: startPluginEntrypoint(
      backendAPI,
      abortSignal,
      config,
      handleWebPkgs,
      failed.reject,
    ),
    done: failed.promise,
  }
}

/**
 * startPluginEntrypoint starts the plugin's capabilities and views and resolves
 * when they are ready. onFailure receives a failure after startup.
 */
async function startPluginEntrypoint(
  backendAPI: BackendAPI,
  abortSignal: AbortSignal,
  config: Config,
  handleWebPkgs: HandleWebPkgsViaPluginAssetsRequest | undefined,
  onFailure: BackendEntrypointFailureFunc,
): Promise<void> {
  // Resolve the plugin's exact runtime identity before starting its modules.
  console.debug('Starting Bldr JS plugin entrypoint...')
  const pluginInfo = await backendAPI.pluginHost.GetPluginInfo({})
  const pluginId = pluginInfo.pluginId
  if (!pluginId?.length) {
    throw new Error('plugin info contained an empty plugin id')
  }

  // Start the controllers assigned to the plugin host.
  const hostConfigSet = config.hostConfigSet
  if (hostConfigSet != null && Object.keys(hostConfigSet).length !== 0) {
    retryWithAbort(
      abortSignal,
      async (abortSignal) => {
        console.debug(
          'starting host config set:',
          JSON.stringify(hostConfigSet),
        )
        backendAPI.pluginHost.ExecController(
          { configSet: { configs: hostConfigSet } },
          abortSignal,
        )
      },
      entrypointRetryOpts('error starting host config set'),
    )
  }

  // Start configured views and wait for backend capability registration.
  const frontendReady = loadWebPlugin(
    backendAPI,
    pluginId,
    abortSignal,
    config,
    handleWebPkgs,
  )
  const capabilityReady = loadBackendEntrypoints(
    backendAPI,
    abortSignal,
    config.backendEntrypoints ?? [],
    undefined,
    onFailure,
  )
  const capabilityRegistrationReady = capabilityReady.then(() =>
    completeInitialCapabilityRegistration(backendAPI, abortSignal),
  )

  // Report QuickJS readiness in stages and forward worker setup failures.
  if (isQuickJSRuntime()) {
    const capabilityFailure = capabilityRegistrationReady.then(
      () => Promise.withResolvers<never>().promise,
    )
    await Promise.race([frontendReady, capabilityFailure])
    reportQuickJSReadiness(quickJSPluginFrontendReadyMarker)
    await capabilityRegistrationReady
    reportQuickJSReadiness(quickJSPluginCapabilityReadyMarker)
  } else {
    void frontendReady.catch((err) => {
      if (!abortSignal.aborted) onFailure(err)
    })
    await capabilityRegistrationReady
  }

  // Publish successful initialization after backend capability registration.
  console.info('Bldr JS plugin entrypoint finished initialization.')
  reportQuickJSReadiness(quickJSPluginReadyMarker)
}
