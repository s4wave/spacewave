import { useEffect, useState } from 'react'

// useRenderDelay returns true only after the supplied delay has elapsed since
// the hook mounted (or since the delay changed). Pair with a loading surface
// to suppress the flash of "Loading..." on fast list / grid loads that
// resolve before the delay elapses. Use a render-delay for list and grid
// surfaces where the absence of a loading state for sub-300ms loads feels
// snappier than showing the skeleton. Single-item and wizard surfaces do not
// apply the delay.
export function useRenderDelay(ms = 300): boolean {
  const [ready, setReady] = useState(false)
  useEffect(() => {
    setReady(false)
    const id = setTimeout(() => setReady(true), ms)
    return () => clearTimeout(id)
  }, [ms])
  return ready
}
