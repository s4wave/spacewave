interface DriveReadyResult {
  body: string
  contentReadyMs: number
  hash: string
  quickstartTiming: {
    state?: string
    progressReadyMs?: number
    contentReadyMs?: number
    finishedMs?: number
    error?: string
  } | null
}

type DriveReadyGlobals = typeof globalThis & {
  __s4waveQuickstartTiming?: DriveReadyResult['quickstartTiming']
  __s4wave_debug?: { quickstartTiming?: DriveReadyResult['quickstartTiming'] }
}

// Polls the drive viewer DOM until the root file listing is visible. Uses
// requestAnimationFrame for frame-synced polling with a deadline.
export default async function (args: {
  deadlineMs: number
}): Promise<DriveReadyResult> {
  return new Promise((resolve, reject) => {
    const deadline = Date.now() + args.deadlineMs
    const readBrowserText = () => {
      const root = document.querySelector('[data-testid="unixfs-browser"]')
      if (!root) {
        return ''
      }
      return root.textContent ?? ''
    }
    const hasDriveWelcome = () =>
      !!document.querySelector('[data-testid="drive-welcome"]')
    const hasDriveInviteCTA = () =>
      !!document.querySelector('[data-testid="drive-invite-cta"]')
    const tick = () => {
      const text = readBrowserText()
      if (text.includes('getting-started.md')) {
        const store = globalThis as DriveReadyGlobals
        const timing =
          store.__s4waveQuickstartTiming ??
          store.__s4wave_debug?.quickstartTiming ??
          null
        resolve({
          body: text.slice(0, 2000),
          contentReadyMs: Math.round(performance.now()),
          hash: window.location.hash,
          quickstartTiming:
            timing ?
              {
                state: timing.state,
                progressReadyMs: timing.progressReadyMs,
                contentReadyMs: timing.contentReadyMs,
                finishedMs: timing.finishedMs,
                error: timing.error,
              }
            : null,
        })
        return
      }
      if (Date.now() > deadline) {
        reject(
          new Error(
            `drive viewer golden path did not appear (hash=${window.location.hash}, welcome=${hasDriveWelcome()}, invite=${hasDriveInviteCTA()}, body=${readBrowserText().slice(0, 500)})`,
          ),
        )
        return
      }
      requestAnimationFrame(tick)
    }
    tick()
  })
}
