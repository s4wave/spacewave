async function waitForRouteCommit(): Promise<void> {
  await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
  await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
}

// Navigates the client-side hash route without reloading the page.
// Waits for the hashchange event before resolving.
export default async function (args: { targetHash: string }): Promise<null> {
  return new Promise((resolve) => {
    if (window.location.hash === args.targetHash) {
      void waitForRouteCommit().then(() => resolve(null))
      return
    }
    const onHashChange = () => {
      if (window.location.hash !== args.targetHash) {
        return
      }
      window.removeEventListener('hashchange', onHashChange)
      void waitForRouteCommit().then(() => resolve(null))
    }
    window.addEventListener('hashchange', onHashChange)
    window.location.hash = args.targetHash
  })
}
