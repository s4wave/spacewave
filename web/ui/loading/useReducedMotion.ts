import { useSyncExternalStore } from 'react'

const reducedMotionQuery = '(prefers-reduced-motion: reduce)'

function getReducedMotionQuery(): MediaQueryList | null {
  if (
    typeof window === 'undefined' ||
    typeof window.matchMedia !== 'function'
  ) {
    return null
  }
  return window.matchMedia(reducedMotionQuery)
}

function getReducedMotionSnapshot(): boolean {
  return getReducedMotionQuery()?.matches ?? false
}

function subscribeReducedMotion(callback: () => void): () => void {
  const query = getReducedMotionQuery()
  if (!query) return () => {}

  query.addEventListener('change', callback)
  return () => {
    query.removeEventListener('change', callback)
  }
}

export function useReducedMotion(): boolean {
  return useSyncExternalStore(
    subscribeReducedMotion,
    getReducedMotionSnapshot,
    () => false,
  )
}
