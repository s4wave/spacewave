// Import types generated from protobuf definitions.
import type { BackendAPI, BackendEntrypointFunc } from '@aptre/bldr-sdk'
import { BackendEntrypoint, FrontendEntrypoint } from './compiler.pb.js'

// Defines the list of backend entrypoints to load.
// Populated by the Bldr JS compiler via esbuild define.
// Value is injected as a literal array object (or undefined if empty).
declare const __BLDR_BACKEND_ENTRYPOINTS__: BackendEntrypoint[] | undefined

// Defines the list of frontend entrypoints.
// Populated by the Bldr JS compiler via esbuild define.
// Value is injected as a literal array object (or undefined if empty).
declare const __BLDR_FRONTEND_ENTRYPOINTS__: FrontendEntrypoint[] | undefined

/**
 * Main execution function for the plugin entrypoint.
 * Loads and executes configured backend and frontend modules.
 */
export default async function main(backendAPI: BackendAPI) {
  console.debug('Starting Bldr JS plugin entrypoint...')

  // Load backend entrypoints directly from the defined constant.
  // Use '?? []' to default to an empty array if the constant is undefined.
  const backendEntrypoints = __BLDR_BACKEND_ENTRYPOINTS__ ?? []
  // backendPromises stores promises returned by backend entrypoint functions.
  const backendPromises: Promise<void>[] = []
  if (backendEntrypoints.length > 0) {
    console.debug(`Loading ${backendEntrypoints.length} backend entrypoints...`)
    for (const entrypoint of backendEntrypoints) {
      // Ensure entrypoint and importPath are valid before proceeding.
      if (!entrypoint?.importPath) {
        console.warn(
          `Skipping invalid backend entrypoint object: ${JSON.stringify(entrypoint)}`,
        )
        continue
      }
      const importPath = entrypoint.importPath
      // Default to 'default' export if import_name is not specified.
      const importName = entrypoint.importName || 'default'
      console.debug(
        `Importing backend module: ${importPath}#${importName || 'default'}`,
      )
      try {
        // The import path is relative to the assets FS root (e.g., /p/{plugin-id}/a/).
        // Example: vite/backend/index.js or esb/backend/index.js
        // The host environment must resolve these paths relative to the assets base URL.
        const mod = await import(/* @vite-ignore */ importPath) // note: we use esbuild to bundle this, but let's keep vite-ignore anyway.
        const modFunc: BackendEntrypointFunc = mod[importName]
        if (mod && typeof modFunc === 'function') {
          console.debug(
            `Executing backend entrypoint: ${importPath}#${importName}`,
          )

          // Execute the function, passing the PluginAPI. Expecting a Promise or void.
          // Store the promise to be awaited later with Promise.all.
          // Wrap the call in Promise.resolve() to handle both Promise<void> and void return values.
          backendPromises.push(
            Promise.resolve(modFunc(backendAPI)).then(() => {
              console.debug(
                `Backend entrypoint finished: ${importPath}#${importName}`,
              )
            }),
          )
        } else {
          console.error(
            `Backend entrypoint function '${importName}' not found or not a function in module: ${importPath}`,
          )
        }
      } catch (error) {
        const errMsg = `Failed to load or execute backend entrypoint ${importPath}#${importName}`
        console.error(`${errMsg}: ${error instanceof Error ? error.message : error}`)
        console.error(errMsg, error) // Also log full error to console for details.
      }
    }
  } else {
    console.debug('No backend entrypoints configured.')
  }

  // Wait for all backend entrypoint promises to resolve.
  if (backendPromises.length > 0) {
    console.debug(
      `Waiting for ${backendPromises.length} backend entrypoints to complete...`,
    )
    try {
      await Promise.all(backendPromises)
      console.debug('All backend entrypoints completed successfully.')
    } catch (error) {
      // Note: Promise.all rejects immediately if any promise rejects.
      // Individual errors during loading/execution are already logged above.
      // This catch block handles errors from the Promise.all itself,
      // potentially related to how promises were handled or aggregated.
      const errMsg = 'Error occurred while waiting for backend entrypoints to complete'
      console.error(`${errMsg}: ${error instanceof Error ? error.message : error}`)
      console.error(errMsg, error)
    }
  }

  // Load frontend entrypoints directly from the defined constant.
  // Use '?? []' to default to an empty array if the constant is undefined.
  const frontendEntrypoints = __BLDR_FRONTEND_ENTRYPOINTS__ ?? []
  if (frontendEntrypoints.length > 0) {
    console.debug(
      `Loading ${frontendEntrypoints.length} frontend entrypoints...`,
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
      console.info(
        `Frontend entrypoint configured (but not loaded): ${entrypoint.importPath}`,
      )
    }
  } else {
    console.debug('No frontend entrypoints configured.')
  }

  console.info('Bldr JS plugin entrypoint finished initialization.')
}
