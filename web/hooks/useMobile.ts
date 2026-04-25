import { useEffect, useState } from 'react'

const MOBILE_BREAKPOINT = 768

// getIsMobile computes the mobile state based on window width.
function getIsMobile(): boolean {
  return typeof window !== 'undefined' && window.innerWidth < MOBILE_BREAKPOINT
}

export function useIsMobile() {
  // Initialize with computed value to avoid setState in effect
  const [isMobile, setIsMobile] = useState<boolean>(getIsMobile)

  useEffect(() => {
    const mql = window.matchMedia(`(max-width: ${MOBILE_BREAKPOINT - 1}px)`)
    const onChange = () => {
      setIsMobile(window.innerWidth < MOBILE_BREAKPOINT)
    }
    mql.addEventListener('change', onChange)
    return () => mql.removeEventListener('change', onChange)
  }, [])

  return isMobile
}
