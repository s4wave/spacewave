// Import types generated from protobuf definitions.
import { Client } from 'starpc'
import type { BackendAPI, BackendEntrypointFunc } from '@aptre/bldr-sdk'
import { BackendEntrypoint, FrontendEntrypoint } from './compiler.pb.js'
import { ConfigSet } from '@go/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.js'
import {
  retryWithAbort,
  SetHtmlLinksRequest,
  SetRenderModeRequest,
} from '@aptre/bldr'
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

/**
 * Loads and executes a single backend entrypoint module.
 * @param entrypoint - The backend entrypoint configuration.
 * @param backendAPI - The backend API object to pass to the entrypoint function.
 * @param abortSignal - The abort signal to pass to the entrypoint function.
 * @returns A promise that resolves when the entrypoint function completes, or rejects on error.
 */
async function executeBackendEntrypoint(
  entrypoint: BackendEntrypoint,
  backendAPI: BackendAPI,
  abortSignal: AbortSignal,
): Promise<void> {
  // Ensure entrypoint and importPath are valid before proceeding.
  if (!entrypoint?.importPath) {
    console.warn(
      `Skipping invalid backend entrypoint object: ${JSON.stringify(entrypoint)}`,
    )
    // Return a resolved promise for invalid entrypoints to not break Promise.all
    return Promise.resolve()
  }

  const importPath = entrypoint.importPath
  // Default to 'default' export if import_name is not specified.
  const importName = entrypoint.importName || 'default'
  const entrypointId = `${importPath}#${importName}`

  console.debug(`Importing backend module: ${entrypointId}`)
  try {
    // The import path is relative to the assets FS root (e.g., /p/{plugin-id}/a/).
    // Example: vite/backend/index.js or esb/backend/index.js
    // The host environment must resolve these paths relative to the assets base URL.
    const mod = await import(/* @vite-ignore */ importPath) // note: we use esbuild to bundle this, but let's keep vite-ignore anyway.
    const modFunc: BackendEntrypointFunc = mod[importName]

    if (typeof modFunc !== 'function') {
      console.error(
        `Backend entrypoint function '${importName}' not found or not a function in module: ${importPath}`,
      )
      // Treat as resolved to avoid breaking Promise.all for other valid entrypoints
      return Promise.resolve()
    }

    console.debug(`Executing backend entrypoint: ${entrypointId}`)
    // Execute the function, passing the PluginAPI.
    // Wrap the call in Promise.resolve() to handle both Promise<void> and void return values.
    // Chain a .then() to log completion.
    return Promise.resolve(modFunc(backendAPI, abortSignal)).then(() => {
      console.debug(`Backend entrypoint finished: ${entrypointId}`)
    })
  } catch (error) {
    logError(
      `Failed to load or execute backend entrypoint ${entrypointId}`,
      error,
    )
    return Promise.reject(error)
  }
}

/**
 * Loads and executes all configured backend entrypoints.
 * @param backendAPI - The backend API object.
 */
async function loadBackendEntrypoints(
  backendAPI: BackendAPI,
  abortSignal: AbortSignal,
): Promise<void> {
  // Load backend entrypoints directly from the defined constant.
  const backendEntrypoints = __BLDR_BACKEND_ENTRYPOINTS__ ?? []

  if (backendEntrypoints.length === 0) {
    console.debug('No backend entrypoints configured.')
    return
  }

  console.debug(`Loading ${backendEntrypoints.length} backend entrypoints...`)

  // backendPromises stores promises returned by backend entrypoint functions.
  const backendPromises: Promise<void>[] = backendEntrypoints.map(
    (entrypoint) =>
      // Wrap execution in a promise chain to catch individual errors
      // without stopping the loading of other entrypoints immediately.
      // We still want Promise.all to report the aggregate failure.
      executeBackendEntrypoint(entrypoint, backendAPI, abortSignal),
  )

  // Wait for all backend entrypoint promises to settle (resolve or reject).
  console.debug(
    `Waiting for ${backendPromises.length} backend entrypoints to complete...`,
  )
  try {
    // Promise.all will reject immediately if any promise rejects.
    await Promise.all(backendPromises)
    console.debug('All backend entrypoints completed successfully.')
  } catch (error) {
    logError(`One or more backend entrypoints threw errors`, error)
  }
}

/**
 * Loads and executes all configured web packages.
 */
async function loadWebPkgs(
  ourPluginID: string,
  webPlugin: WebPlugin,
  abortSignal: AbortSignal,
): Promise<void> {
  const webPkgsIDs = __BLDR_HANDLE_WEB_PKGS__?.webPkgIdList
  if (!webPkgsIDs?.length) {
    console.debug('No web pkgs configured.')
    return
  }

  console.debug(`Processing ${webPkgsIDs.length} web pkgs...`)

  const request = HandleWebPkgsViaPluginAssetsRequest.clone(
    __BLDR_HANDLE_WEB_PKGS__,
  )!
  request.handlePluginId = ourPluginID

  await retryWithAbort(abortSignal, async (signal) => {
    const response = webPlugin.HandleWebPkgsViaPluginAssets(request, signal)
    for await (const result of response) {
      if (result.body?.case !== 'ready') continue
      const isReady = result.body.value || false
      if (isReady) {
        console.debug(
          `Configured ${webPkgsIDs.length} web pkgs via web plugin.`,
        )
      } else {
        console.debug('Web plugin is not ready yet.')
      }
    }
  })
}

/**
 * Loads and executes all configured frontend entrypoints.
 */
async function loadFrontendEntrypoints(
  backendAPI: BackendAPI,
  ourPluginID: string,
  webPlugin: WebPlugin,
  abortSignal: AbortSignal,
): Promise<void> {
  // Load frontend entrypoints directly from the defined constant.
  // Use '?? []' to default to an empty array if the constant is undefined.
  const frontendEntrypoints = __BLDR_FRONTEND_ENTRYPOINTS__ ?? []
  if (frontendEntrypoints.length === 0) {
    console.debug('No frontend entrypoints configured.')
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
    const setRenderModeRequestBin =
      entrypoint.setRenderMode ?
        SetRenderModeRequest.toBinary(entrypoint.setRenderMode)
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
    const setHtmlLinksRequestBin =
      entrypoint.setHtmlLinks ?
        SetHtmlLinksRequest.toBinary(entrypoint.setHtmlLinks)
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
    return
  }

  const handlersRequest: HandleWebViewViaHandlersRequest = {
    config: { handlers },
  }
  console.debug(
    `Configuring ${handlers.length} web view handlers: ${HandleWebViewViaHandlersRequest.toJsonString(handlersRequest)}`,
  )

  await retryWithAbort(abortSignal, async (signal) => {
    const response = webPlugin.HandleWebViewViaHandlers(handlersRequest, signal)
    for await (const result of response) {
      if (result.body?.case !== 'ready') continue
      const isReady = result.body.value || false
      if (isReady) {
        console.debug(
          `Configured ${handlers.length} web view handlers via web plugin.`,
        )
      } else {
        console.debug('Web plugin is not ready yet.')
      }
    }
  })
}

/**
 * Load the web plugin and the frontend entrypoints if any are configured.
 */
function loadWebPlugin(
  backendAPI: BackendAPI,
  ourPluginID: string,
  abortSignal: AbortSignal,
): void {
  // Load the web plugin.
  const webPluginID = __BLDR_WEB_PLUGIN_ID__ ?? ''
  if (!webPluginID?.length) {
    console.debug(
      'Skipping frontend entrypoints as no webPluginId was configured.',
    )
    return
  }

  console.debug(`Loading web plugin with ID: ${webPluginID}`)
  let pluginAbort: AbortController | undefined = undefined
  function startPluginSetup(signal: AbortSignal) {
    if (pluginAbort) {
      return
    }
    pluginAbort = createAbortController(signal)
    retryWithAbort(
      pluginAbort.signal,
      async (signal) => {
        const openStream = backendAPI.buildPluginOpenStream(webPluginID)
        const srpcClient = new Client(openStream)
        const client = new WebPluginClient(srpcClient)
        await Promise.all([
          loadFrontendEntrypoints(backendAPI, ourPluginID, client, signal),
          loadWebPkgs(ourPluginID, client, signal),
        ])
      },
      {
        errorCb: (err) => {
          logError('error loading frontend entrypoints', err)
        },
      },
    )
  }

  retryWithAbort(abortSignal, async (signal) => {
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
  })
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
    retryWithAbort(abortSignal, async (abortSignal) => {
      console.debug('starting host config set:', JSON.stringify(hostConfigSet))
      backendAPI.pluginHost.ExecController(
        { configSet: { configs: hostConfigSet } },
        abortSignal,
      )
    })
  }

  // Load and execute backend entrypoints.
  await loadBackendEntrypoints(backendAPI, abortSignal)

  // Process frontend entrypoints (currently just logs them).
  loadWebPlugin(backendAPI, pluginId, abortSignal)

  console.info('Bldr JS plugin entrypoint finished initialization.')
}
