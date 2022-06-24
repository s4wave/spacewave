// ShutdownCallback is called when closing the page.
export type ShutdownCallback = () => void

// DisposeCallback is called to remove the callback from the page.
export type DisposeCallback = () => void

// window is the global scope.
declare let window: Window

// addShutdownCallback attempts to add a callback when the context closes.
// returns a function to remove the callback.
export function addShutdownCallback(
  cb: ShutdownCallback
): DisposeCallback | null {
  if (window && window.addEventListener) {
    const windowListener = (e: BeforeUnloadEvent) => {
      cb()
      delete e['returnValue']
    }
    window.addEventListener('beforeunload', windowListener)
    return () => {
      window.removeEventListener('beforeunload', windowListener)
    }
  }

  // no way to add a shutdown callback, return nothing.
  return null
}
