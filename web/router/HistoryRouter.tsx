import {
  createContext,
  use,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import type { ReactNode } from 'react'
import { RouterProvider, type To } from './router.js'

// Maximum number of history entries to keep
const MAX_HISTORY_SIZE = 50

// HistoryState holds the navigation history stack and current position.
interface HistoryState {
  stack: string[]
  index: number
}

// HistoryContextType provides history navigation controls.
interface HistoryContextType {
  canGoBack: boolean
  canGoForward: boolean
  enterFlow: (key: string) => void
  exitFlow: (key: string) => void
  goBack: () => void
  goForward: () => void
}

const HistoryContext = createContext<HistoryContextType | null>(null)

interface HistoryRouterProps {
  children: ReactNode
  path: string
  onNavigate: (to: To) => void
}

// HistoryRouter wraps RouterProvider with back/forward history tracking.
export function HistoryRouter({
  children,
  path,
  onNavigate,
}: HistoryRouterProps) {
  // Use ref to track history state to avoid re-renders on history changes
  const historyRef = useRef<HistoryState>({ stack: [path], index: 0 })
  const flowEntriesRef = useRef(new Map<string, string>())
  // Track the last path we saw to detect external navigation
  const lastPathRef = useRef(path)
  // Flag to skip pushing when doing history navigation
  const isHistoryNavRef = useRef(false)
  const onNavigateRef = useRef(onNavigate)
  // eslint-disable-next-line react-hooks/refs
  onNavigateRef.current = onNavigate

  const [canGoBack, setCanGoBack] = useState(false)
  const [canGoForward, setCanGoForward] = useState(false)

  // Update history when path changes externally
  useEffect(() => {
    if (path === lastPathRef.current) return
    lastPathRef.current = path

    if (isHistoryNavRef.current) {
      isHistoryNavRef.current = false
    } else {
      const history = historyRef.current
      const newStack = [...history.stack.slice(0, history.index + 1), path]
      if (newStack.length > MAX_HISTORY_SIZE) {
        newStack.splice(0, newStack.length - MAX_HISTORY_SIZE)
      }
      historyRef.current = {
        stack: newStack,
        index: newStack.length - 1,
      }
    }
    for (const [key, entryPath] of flowEntriesRef.current) {
      if (path === key || path === entryPath) {
        flowEntriesRef.current.delete(key)
      }
    }

    setCanGoBack(historyRef.current.index > 0)
    setCanGoForward(
      historyRef.current.index < historyRef.current.stack.length - 1,
    )
  }, [path])

  const goBack = useCallback(() => {
    const history = historyRef.current
    if (history.index <= 0) return

    const newIndex = history.index - 1
    const targetPath = history.stack[newIndex]
    if (!targetPath) return

    historyRef.current = { ...history, index: newIndex }
    isHistoryNavRef.current = true
    setCanGoBack(newIndex > 0)
    setCanGoForward(newIndex < history.stack.length - 1)
    onNavigateRef.current({ path: targetPath })
  }, [])

  const goBackTo = useCallback((targetPath: string) => {
    const history = historyRef.current
    const newIndex = history.stack.lastIndexOf(targetPath, history.index - 1)

    if (newIndex < 0) {
      const stack = [...history.stack]
      stack[history.index] = targetPath
      historyRef.current = { stack, index: history.index }
      isHistoryNavRef.current = true
      onNavigateRef.current({ path: targetPath, replace: true })
      return
    }

    historyRef.current = { ...history, index: newIndex }
    isHistoryNavRef.current = true
    setCanGoBack(newIndex > 0)
    setCanGoForward(newIndex < history.stack.length - 1)
    onNavigateRef.current({ path: targetPath })
  }, [])

  const enterFlow = useCallback(
    (key: string) => {
      if (flowEntriesRef.current.has(key)) return

      const history = historyRef.current
      const entryPath =
        path !== lastPathRef.current
          ? history.stack[history.index]
          : history.stack[history.index - 1]
      flowEntriesRef.current.set(key, entryPath ?? key)
    },
    [path],
  )

  const exitFlow = useCallback(
    (key: string) => {
      const targetPath = flowEntriesRef.current.get(key) ?? key
      flowEntriesRef.current.delete(key)
      goBackTo(targetPath)
    },
    [goBackTo],
  )

  const goForward = useCallback(() => {
    const history = historyRef.current
    if (history.index >= history.stack.length - 1) return

    const newIndex = history.index + 1
    const targetPath = history.stack[newIndex]
    if (!targetPath) return

    historyRef.current = { ...history, index: newIndex }
    isHistoryNavRef.current = true
    setCanGoBack(newIndex > 0)
    setCanGoForward(newIndex < history.stack.length - 1)
    onNavigateRef.current({ path: targetPath })
  }, [])

  const historyValue = useMemo(
    () => ({
      canGoBack,
      canGoForward,
      enterFlow,
      exitFlow,
      goBack,
      goForward,
    }),
    [canGoBack, canGoForward, enterFlow, exitFlow, goBack, goForward],
  )

  return (
    <HistoryContext.Provider value={historyValue}>
      <RouterProvider path={path} onNavigate={onNavigate}>
        {children}
      </RouterProvider>
    </HistoryContext.Provider>
  )
}

// useHistory returns history navigation controls, or null if not within a HistoryRouter.
export function useHistory(): HistoryContextType | null {
  return use(HistoryContext)
}
