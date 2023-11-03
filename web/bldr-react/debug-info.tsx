import React, {
  createContext,
  useContext,
  useState,
  useEffect,
  ReactNode,
  FC,
  useRef,
  useMemo,
} from 'react'

type DebugInfoContextType = {
  addDebugInfo: (info: ReactNode) => string
  removeDebugInfo: (id: string) => void
  subscribeDebugInfo: (callback: (info: ReactNode[]) => void) => () => void
}

const DebugInfoContext = createContext<DebugInfoContextType | undefined>(
  undefined,
)

const DebugInfoProvider: FC<{ children: ReactNode }> = ({ children }) => {
  const [debugInfo, setDebugInfo] = useState<{ id: string; info: ReactNode }[]>(
    [],
  )
  const idCounter = useRef(0)
  const subscribers = useRef<((info: ReactNode[]) => void)[]>([])

  const notifySubscribers = () => {
    subscribers.current.forEach((callback) =>
      callback(debugInfo.map((di) => di.info)),
    )
  }

  const addDebugInfo = (info: ReactNode): string => {
    const id = (idCounter.current++).toString()
    setDebugInfo((prevInfo) => [...prevInfo, { id, info }])
    return id
  }

  const removeDebugInfo = (id: string) => {
    setDebugInfo((prevInfo) => prevInfo.filter((i) => i.id !== id))
  }

  const subscribeDebugInfo = (callback: (info: ReactNode[]) => void) => {
    subscribers.current.push(callback)
    return () => {
      subscribers.current = subscribers.current.filter(
        (sub) => sub !== callback,
      )
    }
  }

  const contextValue = useMemo(
    () => ({
      addDebugInfo,
      removeDebugInfo,
      subscribeDebugInfo,
    }),
    [],
  )

  useEffect(() => {
    notifySubscribers()
  }, [debugInfo])

  return (
    <DebugInfoContext.Provider value={contextValue}>
      {children}
    </DebugInfoContext.Provider>
  )
}

const DebugInfo: FC<{ children?: ReactNode }> = ({ children }) => {
  useDebugInfo(children)
  return null
}

const useDebugInfo = (info?: ReactNode) => {
  const context = useContext(DebugInfoContext)

  useEffect(() => {
    let id: string | null = null
    if (context && info) {
      id = context.addDebugInfo(info)
    }
    return () => {
      if (id && context) context.removeDebugInfo(id)
    }
  }, [info, context])
}

const DebugInfoDisplay: FC = () => {
  const [localDebugInfo, setLocalDebugInfo] = useState<React.ReactNode[]>([])
  const context = useContext(DebugInfoContext)

  useEffect(() => {
    if (context) {
      const unsubscribe = context.subscribeDebugInfo(setLocalDebugInfo)
      return () => {
        unsubscribe()
      }
    }
  }, [context])

  if (!localDebugInfo.length) {
    return null
  }

  const debugInfoStyle = {
    fontFamily: 'monospace',
    background: 'rgba(0, 0, 0, 0.8)',
    color: 'white',
    fontSize: '12px',
    padding: '0.5rem',
    margin: '1rem',
    maxWidth: '33%',
    minWidth: '10rem',
    overflow: 'auto',
    overflowWrap: 'break-word',
    boxShadow: '0 0 0.5rem 0 rgba(0, 0, 0, 0.2)',
    borderRadius: 0,
    position: 'absolute',
    userSelect: 'none',
    top: 0,
    right: 0,
    zIndex: 1000,
  }

  return (
    <div style={debugInfoStyle}>
      {localDebugInfo.map((info, index) => (
        <p
          style={
            index !== 0
              ? { margin: '0.33rem 0', marginBlockEnd: 0 }
              : { margin: 0 }
          }
          key={index.toString()}
        >
          {info}
        </p>
      ))}
    </div>
  )
}

export {
  DebugInfo,
  DebugInfoContext,
  DebugInfoDisplay,
  DebugInfoProvider,
  useDebugInfo,
}
