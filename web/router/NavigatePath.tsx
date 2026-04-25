import { useEffect, useRef } from 'react'
import { resolvePath, useNavigate, usePath, type To } from './router.js'

interface NavigatePathProps {
  to: string | To
  replace?: boolean
}

/**
 * NavigatePath component that performs navigation when mounted
 * or when the "to" prop changes
 */
export function NavigatePath({ to, replace }: NavigatePathProps) {
  const navigate = useNavigate()
  const path = usePath()
  const pendingTargetRef = useRef<string | null>(null)

  useEffect(() => {
    const toObj =
      typeof to === 'string' ? { path: to, replace } : { ...to, replace }
    const targetPath = resolvePath(path, toObj)
    if (targetPath === path) {
      pendingTargetRef.current = null
      return
    }
    // Guard repeated redirects while the router is still converging on the same
    // destination and the navigate callback identity changes across renders.
    if (pendingTargetRef.current === targetPath) {
      return
    }
    pendingTargetRef.current = targetPath
    navigate(toObj)
  }, [path, to, replace, navigate])

  return null
}
