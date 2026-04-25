import React from 'react'
import { Router } from './router.js'
import { useHashPath, useNavigateHandler } from './hash.js'

/**
 * HashRouter component that provides hash-based routing
 * using the Router component internally
 */
export function HashRouter({ children }: { children: React.ReactNode }) {
  const [path, setPath] = useHashPath()
  const handleNavigate = useNavigateHandler(path, setPath)

  return (
    <Router path={path} onNavigate={handleNavigate}>
      {children}
    </Router>
  )
}
