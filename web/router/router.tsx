import { cleanPath, joinPath, splitPath } from '@aptre/bldr'
import React, {
  createContext,
  useContext,
  useCallback,
  useMemo,
  useRef,
  Fragment,
  isValidElement,
  ReactNode,
  FC,
  ReactElement,
  Children,
} from 'react'

/**
 * Type definition for the Router context.
 */
export interface RouterContextType {
  /** The current path */
  path: string
  /** The current route parameters */
  params: Record<string, string>
  /** Function to navigate to a new path */
  navigate: (to: To) => void
  /** Array of parent paths in the routing hierarchy */
  parentPaths: string[]
}

/**
 * Create the Router context.
 */
export const RouterContext = createContext<RouterContextType | null>(null)

/**
 * Resolves a relative path to an absolute path based on the current path.
 *
 * @param current - The current absolute path.
 * @param to - The path to resolve, which can be absolute or relative.
 * @returns The resolved absolute path.
 */
/**
 * Type definition for navigation destination
 */
export interface To {
  /** The path to navigate to */
  path: string
  /** Whether to replace the current history entry */
  replace?: boolean
}

/**
 * Resolves a relative path to an absolute path based on the current path.
 */
export const resolvePath = (current: string, to: To): string => {
  const toPath = to.path

  // Handle empty paths
  const { pathParts: currSegments } = splitPath(current)
  if (!toPath) {
    return cleanPath(joinPath([...currSegments], true))
  }

  const { pathParts: toSegments, isAbsolute: toAbsolute } = splitPath(toPath)
  if (toAbsolute) {
    return cleanPath(joinPath([...toSegments], true))
  }

  return cleanPath(joinPath([...currSegments, ...toSegments], true))
}

function decodeRoutePart(part: string): string {
  try {
    return decodeURIComponent(part)
  } catch {
    return part
  }
}

/**
 * Matches a route pattern against a path and extracts parameters.
 *
 * @param pattern - The route pattern, which may include parameters starting with ':' and wildcard '*'.
 * @param path - The current path to match against.
 * @returns The extracted parameters if the pattern matches, or null if it doesn't.
 */
const matchRoute = (
  pattern: string,
  path: string,
): Record<string, string> | null => {
  const trimSlashes = (p: string) => p.replace(/^\/+|\/+$/g, '')
  const parts = trimSlashes(pattern).split('/')
  const pathParts = trimSlashes(path).split('/')

  const params: Record<string, string> = {}

  let i = 0
  for (; i < parts.length; i++) {
    const patternPart = parts[i]
    const pathPart = pathParts[i]

    if (patternPart === '*') {
      // Wildcard matches the rest of the path.
      params['*'] = decodeRoutePart(pathParts.slice(i).join('/'))
      return params
    }

    if (pathPart === undefined) {
      // The path is shorter than the pattern.
      return null
    }

    if (patternPart.startsWith(':')) {
      // Parameter segment; extract the value.
      params[patternPart.slice(1)] = decodeRoutePart(pathPart)
    } else if (patternPart !== pathPart) {
      // Static segments do not match.
      return null
    }
  }

  if (i < pathParts.length) {
    // The path has extra segments not accounted for in the pattern.
    return null
  }

  return params
}

/**
 * RouterProvider component that provides the routing context to its children.
 *
 * @param children - The child components.
 * @param path - The current path.
 * @param onNavigate - Callback function when navigation occurs.
 */
export const RouterProvider: FC<{
  children: ReactNode
  path: string
  onNavigate: (to: To) => void
}> = ({ children, path, onNavigate }) => {
  const onNavigateRef = useRef(onNavigate)
  // eslint-disable-next-line react-hooks/refs
  onNavigateRef.current = onNavigate

  const navigate = useCallback((to: To) => {
    onNavigateRef.current(to)
  }, [])

  const value = useMemo<RouterContextType>(
    () => ({
      path,
      params: {},
      navigate,
      parentPaths: [],
    }),
    [path, navigate],
  )

  return (
    <RouterContext.Provider value={value}>{children}</RouterContext.Provider>
  )
}

/**
 * Router component that initializes the router context.
 * This is a convenience wrapper around RouterProvider.
 */
export const Router: FC<{
  children: ReactNode
  path: string
  onNavigate: (to: To) => void
}> = ({ children, path, onNavigate }) => {
  return (
    <RouterProvider path={path} onNavigate={onNavigate}>
      {children}
    </RouterProvider>
  )
}

/**
 * Type definition for Route properties.
 */
interface RouteProps {
  /** The path pattern for this route */
  path: string
  /** The children to render when this route matches */
  children: ReactNode
}

// collectRoutes recursively flattens children into Route elements.
// Handles Fragments and wrapper elements whose props.children contain Routes.
function collectRoutes(children: ReactNode): ReactElement<RouteProps>[] {
  const result: ReactElement<RouteProps>[] = []
  for (const child of Children.toArray(children)) {
    if (!isValidElement(child)) continue
    if ((child.props as RouteProps).path !== undefined) {
      result.push(child as ReactElement<RouteProps>)
    } else if (
      child.type === Fragment ||
      (child.props as { children?: ReactNode }).children
    ) {
      result.push(
        ...collectRoutes((child.props as { children?: ReactNode }).children),
      )
    }
  }
  return result
}

/**
 * Routes component that renders the first child Route that matches the current path.
 *
 * @param children - An array of Route components.
 */
export const Routes: FC<{
  children: ReactNode
  path?: string
  fullPath?: boolean
}> = ({ children, path: pathProp, fullPath }) => {
  const router = useRouter()
  const effectivePath =
    fullPath ?
      (pathProp ?? router.path)
    : (router.params['*'] ?? pathProp ?? router.path)

  const childElements = collectRoutes(children)
  for (const child of childElements) {
    const { path: routePattern, children } = child.props
    const params = matchRoute(routePattern, effectivePath)
    if (params) {
      // Calculate the basePath by removing the wildcard portion if it exists
      // Calculate the current path segment
      const currentPath =
        params['*'] ?
          effectivePath.slice(0, -params['*'].length - 1) // -1 for the trailing slash
        : effectivePath

      const value = {
        ...router,
        params,
        parentPaths:
          routePattern.endsWith('*') ?
            [...router.parentPaths, currentPath]
          : router.parentPaths,
      }
      return (
        <RouterContext.Provider value={value}>
          {children}
        </RouterContext.Provider>
      )
    }
  }

  return null // No route matched; render nothing or a 'Not Found' component.
}

/**
 * Route component that defines a route pattern and the children to render.
 * This component is not rendered directly.
 *
 * @param path - The route pattern.
 * @param children - The children to render when the route matches.
 */
export const Route: FC<RouteProps> = () => {
  // This component does not render anything itself.
  return null
}

/**
 * Hook to access the routing context.
 *
 * @returns The RouterContextType object containing path, params, and navigate.
 */
export const useRouter = (): RouterContextType => {
  const context = useContext(RouterContext)
  if (!context) {
    throw new Error('useRouter must be used within a RouterProvider')
  }
  return context
}

/**
 * Hook to access the route parameters.
 *
 * @returns An object containing the route parameters.
 */
export const useParams = (): Record<string, string> => {
  return useRouter().params
}

/**
 * Hook to access the current path.
 *
 * @returns The current path as a string.
 */
export const usePath = (): string => {
  return useRouter().path
}

/**
 * Hook to navigate to a new path.
 *
 * @returns A function that accepts a path to navigate to.
 */
export const useNavigate = (): ((to: To) => void) => {
  return useRouter().navigate
}

/**
 * Hook to access the parent paths.
 *
 * @returns Array of parent paths in the routing hierarchy.
 */
export const useParentPaths = (): string[] => {
  return useRouter().parentPaths
}
