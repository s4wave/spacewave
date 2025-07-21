/**
 * createAbortController creates a new AbortController that will be aborted
 * when the parent signal is aborted or when the controller itself is aborted.
 *
 * @param parentSignal - Optional parent signal to inherit abortion from
 * @returns A new AbortController
 */
export function createAbortController(
  parentSignal?: AbortSignal,
): AbortController {
  const controller = new AbortController()

  if (parentSignal) {
    if (parentSignal.aborted) {
      controller.abort()
    } else {
      parentSignal.addEventListener('abort', () => controller.abort(), {
        once: true,
      })
    }
  }

  return controller
}
