import { DependencyList, useEffect, useContext } from 'react'
import { Client } from 'starpc'
import { BldrContext } from './bldr-context.js'

// Destructor is the destructor type from React.
type Destructor = () => void

// useWebViewHostClient builds a client and abort signal for the web view host.
export function useWebViewHostClient(
    effect: (client: Client, abortSignal: AbortSignal) => (void | Destructor),
    deps?: DependencyList,
) {
    const bldrContext = useContext(BldrContext)
    let effectDeps: DependencyList = [bldrContext]
    if (deps) {
        effectDeps = effectDeps.concat(deps)
    }
    useEffect(() => {
        if (!bldrContext?.webDocument || !bldrContext?.webView) {
            return
        }
        const client = bldrContext.webDocument.buildWebViewHostClient(bldrContext.webView.getUuid())
        const cancel = new AbortController()
        const destructor = effect(client, cancel.signal)
        return () => {
            cancel.abort()
            if (destructor) {
                destructor()
            }
        }
    }, effectDeps)
}
