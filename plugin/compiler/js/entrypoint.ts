// Import types generated from protobuf definitions.
import type { BackendAPI, BackendEntrypointFunc } from '@aptre/bldr-sdk'
import { BackendEntrypoint, FrontendEntrypoint } from './compiler.pb.js'
import { ConfigSet } from '@go/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.js'
import { retryWithAbort } from '@aptre/bldr'

// Defines the list of backend entrypoints to load.
declare const __BLDR_BACKEND_ENTRYPOINTS__: BackendEntrypoint[] | undefined

// Defines the list of frontend entrypoints.
declare const __BLDR_FRONTEND_ENTRYPOINTS__: FrontendEntrypoint[] | undefined

// Defines the set of config set to apply to the plugin host.
declare const __BLDR_HOST_CONFIG_SET__: ConfigSet['configs'] | undefined

/**
 * Loads and executes a single backend entrypoint module.
 * @param entrypoint - The backend entrypoint configuration.
 * @param backendAPI - The backend API object to pass to the entrypoint function.
 * @returns A promise that resolves when the entrypoint function completes, or rejects on error.
 */
async function executeBackendEntrypoint(
  entrypoint: BackendEntrypoint,
  backendAPI: BackendAPI,
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
    return Promise.resolve(modFunc(backendAPI)).then(() => {
      console.debug(`Backend entrypoint finished: ${entrypointId}`)
    })
  } catch (error) {
    const errMsg = `Failed to load or execute backend entrypoint ${entrypointId}`
    console.error(
      `${errMsg}: ${error instanceof Error ? error.message : error}`,
    )
    console.error(errMsg, error) // Also log full error object.
    // Propagate the error by returning a rejected promise.
    // This ensures Promise.all below will catch the failure.
    return Promise.reject(error)
  }
}

/**
 * Loads and executes all configured backend entrypoints.
 * @param backendAPI - The backend API object.
 */
async function loadBackendEntrypoints(backendAPI: BackendAPI): Promise<void> {
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
      executeBackendEntrypoint(entrypoint, backendAPI),
  )

  // Wait for all backend entrypoint promises to settle (resolve or reject).
  console.debug(
    `Waiting for ${backendPromises.length} backend entrypoints to complete...`,
  )
  try {
    // Promise.all will reject immediately if any promise rejects.
    await Promise.all(backendPromises)
    console.debug('All backend entrypoints completed successfully.')
  } catch (_error) {
    // Individual errors during loading/execution are already logged by executeBackendEntrypoint.
    // This catch block handles the aggregate failure reported by Promise.all.
    const errMsg =
      'Error occurred while waiting for one or more backend entrypoints to complete'
    // Log a summary error; specific error details were logged earlier.
    console.error(`${errMsg}. See previous logs for details.`)
    // Optionally re-throw or handle the aggregate error further if needed.
    // For now, just logging it is sufficient as individual errors are logged.
  }
}

/**
 * Logs information about configured frontend entrypoints.
 * (Does not load or execute them).
 */
function loadFrontendEntrypoints(): void {
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
  // TODO: Implement frontend entrypoint loading mechanism
  for (const entrypoint of frontendEntrypoints) {
    // Ensure entrypoint and importPath are valid before proceeding.
    if (!entrypoint?.importPath) {
      console.warn(
        `Skipping invalid frontend entrypoint object: ${JSON.stringify(entrypoint)}`,
      )
      continue
    }
    // Currently, just log that they are configured.
    console.info(
      `Frontend entrypoint configured (but not loaded): ${entrypoint.importPath}`,
    )
  }
}

/**
 * Main execution function for the plugin entrypoint.
 * Loads and executes configured backend and frontend modules.
 */
export default async function main(backendAPI: BackendAPI) {
  console.debug('Starting Bldr JS plugin entrypoint...')

  const abortController = new AbortController()
  const abortSignal = abortController.signal

  // Load the plugin info to determine the host volume info (unused currently in Js)
  // const pluginInfo = await backendAPI.pluginHost.GetPluginInfo({})

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
  await loadBackendEntrypoints(backendAPI)

  // Process frontend entrypoints (currently just logs them).
  loadFrontendEntrypoints()

  console.info('Bldr JS plugin entrypoint finished initialization.')
}
