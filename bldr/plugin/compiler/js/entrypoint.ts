// Import types generated from protobuf definitions.
import { Client, isAbortError } from 'starpc'
import {
  isBackendEntrypointFunc,
  isBackendEntrypointLifecycle,
  type BackendAPI,
} from '@aptre/bldr-sdk'
import { BackendEntrypoint, FrontendEntrypoint } from './compiler.pb.js'
import { ConfigSet } from '@go/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.js'
import {
  retryWithAbort,
  SetHtmlLinksRequest,
  SetRenderModeRequest,
} from '@aptre/bldr'
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
} from 'web/plugin/plugin.pb.js'

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
    pluginId,
    importPath.slice('/assets/'.length),
  )
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

function observeBackendEntrypointCompletion(
  entrypointId: string,
  entrypointPromise: Promise<void> | void,
): void {
  Promise.resolve(entrypointPromise)
    .then(() => {
      console.debug(`Backend entrypoint finished: ${entrypointId}`)
    })
    .catch((error: unknown) => {
      logError(`Backend entrypoint failed after startup ${entrypointId}`, error)
    })
}

/**
 * Loads and executes a single backend entrypoint module.
 * @param entrypoint - The backend entrypoint configuration.
 * @param backendAPI - The backend API object to pass to the entrypoint function.
 * @param abortSignal - The abort signal to pass to the entrypoint function.
 * @returns A promise that resolves when the entrypoint function starts, or rejects on startup error.
 */
export async function startBackendEntrypoint(
  entrypoint: BackendEntrypoint,
  backendAPI: BackendAPI,
  abortSignal: AbortSignal,
  loadModule: BackendEntrypointModuleLoader = importBackendEntrypointModule,
): Promise<void> {
  if (!entrypoint?.importPath) {
    throw new Error(
      `Invalid backend entrypoint object: ${JSON.stringify(entrypoint)}`,
    )
  }

  const importPath = resolveBackendEntrypointImportPath(
    entrypoint.importPath,
    backendAPI,
  )
  // Default to 'default' export if import_name is not specified.
  const importName = entrypoint.importName || 'default'
  const entrypointId = `${importPath}#${importName}`

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
      observeBackendEntrypointCompletion(entrypointId, entrypointResult.done)
      return
    }
    observeBackendEntrypointCompletion(entrypointId, entrypointResult)
  } catch (error) {
    logError(
      `Failed to load or start backend entrypoint ${entrypointId}`,
      error,
    )
    return Promise.reject(error)
  }
}

/**
 * Loads and executes all configured backend entrypoints.
 * @param backendAPI - The backend API object.
 */
export async function loadBackendEntrypoints(
  backendAPI: BackendAPI,
  abortSignal: AbortSignal,
  backendEntrypoints: BackendEntrypoint[] = __BLDR_BACKEND_ENTRYPOINTS__ ?? [],
  loadModule: BackendEntrypointModuleLoader = importBackendEntrypointModule,
): Promise<void> {
  if (backendEntrypoints.length === 0) {
    console.debug('No backend entrypoints configured.')
    return
  }

  console.debug(`Loading ${backendEntrypoints.length} backend entrypoints...`)

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
    )
  }
  console.debug('All backend entrypoints started successfully.')
}

/**
 * Loads and executes all configured web packages.
 */
export async function loadWebPkgs(
  ourPluginID: string,
  webPlugin: WebPlugin,
  abortSignal: AbortSignal,
  onReady?: () => void,
  handleWebPkgs:
    | HandleWebPkgsViaPluginAssetsRequest
    | undefined = __BLDR_HANDLE_WEB_PKGS__,
): Promise<void> {
  const webPkgsIDs = handleWebPkgs?.webPkgIdList
  if (!webPkgsIDs?.length) {
    console.debug('No web pkgs configured.')
    onReady?.()
    return
  }

  const request = HandleWebPkgsViaPluginAssetsRequest.clone(handleWebPkgs)!
  request.handlePluginId = ourPluginID

  const { promise, resolve, reject } = Promise.withResolvers<void>()
  let ready = false
  const cleanup = () => {
    abortSignal.removeEventListener('abort', abortReady)
  }
  const resolveReady = () => {
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
 * Loads and executes all configured frontend entrypoints.
 */
async function loadFrontendEntrypoints(
  backendAPI: BackendAPI,
  ourPluginID: string,
  webPlugin: WebPlugin,
  abortSignal: AbortSignal,
  onReady?: () => void,
): Promise<void> {
  // Load frontend entrypoints directly from the defined constant.
  // Use '?? []' to default to an empty array if the constant is undefined.
  const frontendEntrypoints = __BLDR_FRONTEND_ENTRYPOINTS__ ?? []
  if (frontendEntrypoints.length === 0) {
    console.debug('No frontend entrypoints configured.')
    onReady?.()
    return
  }

  console.debug(
    `Processing ${frontendEntrypoints.length} frontend entrypoints...`,
  )

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

      // Override the script path to be /b/pa/{plugin-id}/...
      if (setRenderModeRequest.scriptPath) {
        setRenderModeRequest.scriptPath = backendAPI.utils.pluginAssetHttpPath(
          ourPluginID,
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

      // Override the href paths to be /b/pa/{plugin-id}/...
      if (setHtmlLinksRequest.setLinks) {
        for (const link of Object.values(setHtmlLinksRequest.setLinks)) {
          if (link?.href) {
            link.href = backendAPI.utils.pluginAssetHttpPath(
              ourPluginID,
              link.href,
            )
          }
        }
      }

      pushHandler({ case: 'setHtmlLinks', value: setHtmlLinksRequest })
    }
  }

  if (!handlers.length) {
    console.debug(`No web view handlers were configured.`)
    onReady?.()
    return
  }

  const handlersRequest: HandleWebViewViaHandlersRequest = {
    config: { handlers },
  }
  console.debug(
    `Configuring ${handlers.length} web view handlers: ${HandleWebViewViaHandlersRequest.toJsonString(handlersRequest)}`,
  )

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
): Promise<void> {
  // Load the web plugin.
  const webPluginID = __BLDR_WEB_PLUGIN_ID__ ?? ''
  if (!webPluginID?.length) {
    console.debug(
      'Skipping frontend entrypoints as no webPluginId was configured.',
    )
    return Promise.resolve()
  }

  if (
    !(__BLDR_FRONTEND_ENTRYPOINTS__ ?? []).length &&
    !__BLDR_HANDLE_WEB_PKGS__?.webPkgIdList?.length
  ) {
    console.debug('No frontend handlers or web pkgs configured.')
    return Promise.resolve()
  }

  console.debug(`Loading web plugin with ID: ${webPluginID}`)
  let pluginAbort: AbortController | undefined = undefined
  return new Promise<void>((resolve, reject) => {
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

    function startPluginSetup(signal: AbortSignal) {
      if (pluginAbort) {
        return
      }
      pluginAbort = createAbortController(signal)
      let setupAttempt = 0
      retryWithAbort(
        pluginAbort.signal,
        async (signal) => {
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
            await loadWebPkgs(ourPluginID, client, signal, setupReady)
            await loadFrontendEntrypoints(
              backendAPI,
              ourPluginID,
              client,
              signal,
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
  using rootRef = await backendAPI.resourceClient.accessRootResource()
  const svc = new PluginHostResourceServiceClient(rootRef.client)
  await svc.CompleteInitialCapabilityRegistration({}, abortSignal)
}

/**
 * Main execution function for the plugin entrypoint.
 * Loads and executes configured backend and frontend modules.
 */
export default async function main(
  backendAPI: BackendAPI,
  abortSignal: AbortSignal,
) {
  console.debug('Starting Bldr JS plugin entrypoint...')

  // Load the plugin info
  const pluginInfo = await backendAPI.pluginHost.GetPluginInfo({})
  const pluginId = pluginInfo.pluginId
  if (!pluginId?.length) {
    throw new Error('plugin info contained an empty plugin id')
  }

  // Load and start the hostConfigSet, if any.
  const hostConfigSet = __BLDR_HOST_CONFIG_SET__ ?? undefined
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

  const frontendReady = loadWebPlugin(backendAPI, pluginId, abortSignal)
  const capabilityReady = loadBackendEntrypoints(backendAPI, abortSignal)
  const capabilityRegistrationReady = capabilityReady.then(() =>
    completeInitialCapabilityRegistration(backendAPI, abortSignal),
  )

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
      if (abortSignal.aborted) return
      const globals = globalThis as typeof globalThis & {
        BLDR_PLUGIN_REPORT_RUNTIME_FAILURE?: (err: unknown) => void
      }
      globals.BLDR_PLUGIN_REPORT_RUNTIME_FAILURE?.(err)
    })
    await capabilityRegistrationReady
  }

  console.info('Bldr JS plugin entrypoint finished initialization.')
  reportQuickJSReadiness(quickJSPluginReadyMarker)
}
